// 进程表补充发现（v0.7 补丁）：`claude agents --json` 在 2.1.x 上只报
// background 会话；桌面 app 托管会话（命令行带 --session-id 的 --agent 进程）
// 与交互 TUI（裸 claude 主进程，占着一个控制终端）都在发现口之外，
// 但它们是用户眼前的实时会话，船队计数与列表必须算上。
//
// 识别手段全部来自本机可见状态，不碰 daemon 私有协议：
//   - 桌面会话：命令行 --session-id <uuid> 直接给 sid，transcript 给目录与标题；
//   - TUI：有控制终端的裸 claude 进程，按 cwd + 进程启动时间配对刚写入的
//     transcript 文件；开了终端但还没说过话的会话没有 transcript，只能显示
//     匿名行（pid 当临时身份），它一开口下一轮（30s）就配对成真会话。
package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bestksl/mindport/internal/transcript"
)

// procSession 就是带上进程来源标记的 cliSession（Anon 标记「开了终端
// 但还没说过话」的空会话）。
type procSession = cliSession

// anonSID 是匿名 TUI 在合并快照里用的临时键（前缀 proc-），不可进船台/不可 resume。
func anonSID(pid int) string { return "proc-" + strconv.Itoa(pid) }

func isAnonSID(sid string) bool { return strings.HasPrefix(sid, "proc-") }

var sidFlagRe = regexp.MustCompile(`--session-id[=\s]+([0-9a-fA-F-]{36})`)

// procCandidate 是从 ps 输出里认出的候选进程（纯解析，便于测试）。
type procCandidate struct {
	PID   int
	TTY   string
	Start time.Time
	Args  string
}

// psTimeLayout 是 macOS `ps -lstart` 的格式："Sun Oct 26 21:03:11 2026"，
// 无时区段（注意不是 time.UnixDate，后者含 MST），按本地时区解释。
const psTimeLayout = "Mon Jan _2 15:04:05 2006"

// parsePSOutput 解析 `ps -axo pid=,tty=,lstart=,args=` 输出。
// lstart 固定占 5 段（Mon Jan _2 15:04:05 2006），其后全部归 args。
func parsePSOutput(b []byte) []procCandidate {
	var out []procCandidate
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		start, err := time.ParseInLocation(psTimeLayout, strings.Join(f[2:7], " "), time.Local)
		if err != nil {
			continue
		}
		out = append(out, procCandidate{
			PID: pid, TTY: f[1], Start: start,
			Args: strings.Join(f[7:], " "),
		})
	}
	return out
}

// classifyProc 判断候选是不是发现口之外的实时会话。
// 返回 (sid, ok)：sid 非空=桌面 --agent 直接给了身份；
// sid 空且 ok=true=交互 TUI，身份待 transcript 配对。
func classifyProc(c procCandidate) (sid string, ok bool) {
	if m := sidFlagRe.FindStringSubmatch(c.Args); m != nil {
		// --agent 工作进程是真会话；bg-spare / pty-host / daemon 只是陪跑。
		if strings.Contains(c.Args, "--agent") {
			return strings.ToLower(m[1]), true
		}
		return "", false
	}
	// 裸 claude 主进程：args 只有一个路径段，且占着控制终端（?? 是无终端守护）。
	fields := strings.Fields(c.Args)
	if c.TTY != "??" && len(fields) == 1 && filepath.Base(fields[0]) == "claude" {
		return "", true
	}
	return "", false
}

// procCWDs 一次性取多个进程的工作目录：lsof -p pid,pid -a -d cwd -F pn。
func procCWDs(pids []int) map[int]string {
	out := map[int]string{}
	if len(pids) == 0 {
		return out
	}
	parts := make([]string, len(pids))
	for i, p := range pids {
		parts[i] = strconv.Itoa(p)
	}
	b, err := exec.Command("lsof", "-p", strings.Join(parts, ","), "-a", "-d", "cwd", "-F", "pn").Output()
	if err != nil {
		return out // lsof 对已退出进程会非零退出，已拿到的行仍解析
	}
	cur := 0
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			cur, _ = strconv.Atoi(line[1:])
		case 'n':
			if cur != 0 {
				out[cur] = line[1:]
			}
		}
	}
	return out
}

type sidFile struct {
	sid  string
	dir  string // 编码目录绝对路径
	path string
	mod  time.Time
}

// indexTranscripts 建一份 sid → transcript 文件的全量索引（仅文件名+stat）。
func indexTranscripts(home string) ([]sidFile, error) {
	paths, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := make([]sidFile, 0, len(paths))
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		out = append(out, sidFile{
			sid:  strings.TrimSuffix(filepath.Base(p), ".jsonl"),
			dir:  filepath.Dir(p),
			path: p,
			mod:  st.ModTime(),
		})
	}
	return out, nil
}

// procPending 是认出后的待配对候选：sid 非空=桌面 --agent（身份已知），
// isTUI=交互终端主进程（身份要靠 transcript 配对）。
type procPending struct {
	c     procCandidate
	sid   string
	isTUI bool
}

// scanProcessSessions 跑一次 ps + 配对，返回发现口（known）之外的实时会话。
func scanProcessSessions(home string, known map[string]cliSession) []procSession {
	// lstart 的格式随 LC_TIME 变：中文环境输出「日 10/25 17:46:48 2026」，
	// 固定 5 段英文格式根本解析不出来（静默漏光所有 TUI）。强制 C 区域再解析。
	cmd := exec.Command("ps", "-axo", "pid=,tty=,lstart=,args=")
	cmd.Env = append(os.Environ(), "LC_TIME=C", "LANG=C")
	b, err := cmd.Output()
	if err != nil {
		return nil
	}
	var pend []procPending
	pidSet := map[int]bool{}
	for _, c := range parsePSOutput(b) {
		sid, ok := classifyProc(c)
		if !ok {
			continue
		}
		pidSet[c.PID] = true // 桌面会话在 transcript 尚未落盘时也要靠进程 cwd 兜底
		if sid != "" {
			if _, exists := known[sid]; exists {
				continue // 发现口已报（权威状态优先）
			}
			pend = append(pend, procPending{c: c, sid: sid})
		} else {
			pend = append(pend, procPending{c: c, isTUI: true})
		}
	}
	if len(pend) == 0 {
		return nil
	}

	pids := make([]int, 0, len(pidSet))
	for pid := range pidSet {
		pids = append(pids, pid)
	}
	cwds := procCWDs(pids)

	files, err := indexTranscripts(home)
	if err != nil {
		return nil
	}
	knownSet := make(map[string]bool, len(known))
	for sid := range known {
		knownSet[sid] = true
	}
	return pairProcSessions(home, pend, cwds, files, knownSet)
}

// pairProcSessions 纯配对：桌面会话按 sid 找 transcript（找不到用进程 cwd），
// TUI 在自己 cwd 的编码目录里找进程启动后新写入、未被占用的最新 transcript；
// 都配不上的 TUI 落为匿名行（proc-<pid>）。
func pairProcSessions(home string, pend []procPending, cwds map[int]string, files []sidFile, known map[string]bool) []cliSession {
	byDir := map[string][]sidFile{}
	bySID := map[string]sidFile{}
	for _, f := range files {
		byDir[f.dir] = append(byDir[f.dir], f)
		bySID[f.sid] = f
	}
	taken := map[string]bool{} // 配对占用，防两个 TUI 撞一个文件
	for sid := range known {
		taken[sid] = true
	}
	var out []cliSession

	// 桌面 --agent：sid 已知，transcript 补目录/标题；刚启动还没文件就用进程 cwd。
	for _, p := range pend {
		if p.isTUI {
			continue
		}
		ps := cliSession{
			SessionID: p.sid, ID: shortSid(p.sid), Kind: "interactive", PID: p.c.PID,
			StartedAt: p.c.Start.UnixMilli(),
		}
		if f, ok := bySID[p.sid]; ok {
			taken[p.sid] = true
			fillFromTranscript(&ps, f)
		} else if cwd := cwds[p.c.PID]; cwd != "" {
			ps.CWD = cwd
		}
		if ps.CWD == "" {
			continue
		}
		ps.Status = inferStatus(bySID[p.sid])
		out = append(out, ps)
	}

	// TUI：在自己 cwd 的编码目录里，找进程启动后新写入、且未被占用的最新文件。
	for _, p := range pend {
		if !p.isTUI {
			continue
		}
		cwd := cwds[p.c.PID]
		ps := cliSession{Kind: "interactive", CWD: cwd, PID: p.c.PID, StartedAt: p.c.Start.UnixMilli()}
		if cwd != "" {
			cut := p.c.Start.Add(-90 * time.Second)
			pdir := transcript.ProjectDir(home, cwd)
			var best *sidFile
			for i := range byDir[pdir] {
				f := &byDir[pdir][i]
				if taken[f.sid] || f.mod.Before(cut) {
					continue
				}
				if best == nil || f.mod.After(best.mod) {
					best = f
				}
			}
			if best != nil {
				taken[best.sid] = true
				ps.SessionID = best.sid
				ps.ID = shortSid(best.sid)
				fillFromTranscript(&ps, *best)
			}
		}
		if ps.SessionID == "" {
			ps.Anon = true
			ps.SessionID = anonSID(p.c.PID)
			ps.ID = ps.SessionID
			ps.Status = "waiting" // 刚开的空会话，就是在等人说话
		} else {
			ps.Status = inferStatus(bySID[ps.SessionID])
		}
		out = append(out, ps)
	}

	// 稳定顺序：有身份的按 sid，匿名（还没说话的空会话）垫后
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Anon != out[j].Anon {
			return !out[i].Anon
		}
		return out[i].SessionID < out[j].SessionID
	})
	return out
}

// fillFromTranscript 用 transcript 内容补目录（记录内 cwd 自证）与标题。
func fillFromTranscript(ps *cliSession, f sidFile) {
	m := transcript.MetaPath(f.path)
	if m.Dir != "" {
		ps.CWD = m.Dir
	}
	ps.Name = m.Title
}

// inferStatus 推断发现口之外会话的状态：文件 20s 内还在写且末条是 assistant
// 视为工作中，其余一律「待你输入」（交互会话停着就是在等人说话）。
func inferStatus(f sidFile) string {
	if f.path == "" || time.Since(f.mod) > 20*time.Second {
		return "waiting"
	}
	pg, err := transcript.ReadTail(f.path)
	if err != nil || len(pg.Events) == 0 {
		return "waiting"
	}
	last := pg.Events[len(pg.Events)-1]
	if last.Role == "assistant" {
		return "busy"
	}
	return "waiting"
}
