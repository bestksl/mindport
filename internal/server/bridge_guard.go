// bridge_guard：--resume 起桥前的席位探测。
//
// 网页桥是常驻 claude -p 进程，活着时占着这个 sid 的会话席位；终端里再
// `claude --resume <sid>` 会被 daemon 以「上一个会话没结束」拒之门外。
// 反向同理：终端正开着同一 sid 时，网页若硬起桥，两代 CLI 会双写同一份
// transcript。收割器负责前一个方向（空闲退役），这里负责后一个方向
// （起桥前看看别人在不在），两路合起来保证同一时刻同一 sid 只有一个 writer。
//
// 探测两路并行、全部 fail-open：发现口超时/格式漂移/ps 不可用都按「没人占」
// 放行——绝不能因为探测打盹把说话功能焊死。
package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// holderProbeTimeout：发现口探测的硬超时（正常本地调用是亚秒级）。
const holderProbeTimeout = 4 * time.Second

// resumeSidRe 认命令行里的 --resume <sid> / --session-id=<sid>（发现口漏报兜底）。
var resumeSidRe = regexp.MustCompile(`--(?:resume|session-id)(?:=|\s+)([0-9a-fA-F-]{36})`)

// externalHolder 报告 sid 是否被本桥以外的 claude 客户端占着。
// 返回占用者 PID、给人看的来源描述。查不到/出错一律 ok=false（放行）。
func (b *fleetBridge) externalHolder(sid string) (int, string, bool) {
	if b.testHolder != nil { // 测试替身
		if b.testHolder(sid) {
			return -1, "测试占席客户端", true
		}
		return 0, "", false
	}
	own := b.ownPIDs()

	// 第一路：claude agents 发现口（daemon 权威快照；实测 -p 桥也在其中）。
	if exe := b.srv.claudeExe(); exe != "" {
		ctx, cancel := context.WithTimeout(context.Background(), holderProbeTimeout)
		out, err := exec.CommandContext(ctx, exe, "agents", "--json").Output()
		cancel()
		if err == nil {
			if ss, perr := parseAgents(out); perr == nil {
				for _, s := range ss {
					if s.SessionID == sid && s.PID > 0 && !own[s.PID] && processAlive(s.PID) {
						return s.PID, holderKindText(s.Kind), true
					}
				}
			}
		}
	}

	// 第二路：ps 命令行兜底（TUI resume、发现口延迟/漏报）。
	if pid, who, ok := holderFromPS(sid, own); ok {
		return pid, who, true
	}
	return 0, "", false
}

// ownPIDs 列出本桥当前还活着的全部桥进程 PID，探测时排除自己人。
func (b *fleetBridge) ownPIDs() map[int]bool {
	own := map[int]bool{}
	b.mu.Lock()
	fps := make([]*fleetProc, 0, len(b.procs))
	for _, fp := range b.procs {
		fps = append(fps, fp)
	}
	b.mu.Unlock()
	for _, fp := range fps {
		fp.mu.Lock()
		if fp.cmd != nil && fp.cmd.Process != nil && fp.state != bridgeDead {
			own[fp.cmd.Process.Pid] = true
		}
		fp.mu.Unlock()
	}
	return own
}

func holderKindText(kind string) string {
	switch kind {
	case "background":
		return "后台 Claude 任务"
	default:
		return "终端/交互客户端"
	}
}

// holderFromPS 扫进程命令行，找还活着、且不是本桥自己的 claude --resume/
// --session-id <sid> 进程。
func holderFromPS(sid string, own map[int]bool) (int, string, bool) {
	cmd := exec.Command("ps", "-axo", "pid=,args=")
	cmd.Env = append(os.Environ(), "LC_TIME=C", "LANG=C")
	out, err := cmd.Output()
	if err != nil {
		return 0, "", false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sp := strings.IndexAny(line, " \t")
		if sp < 0 {
			continue
		}
		pid, perr := strconv.Atoi(strings.TrimSpace(line[:sp]))
		if perr != nil || pid <= 0 || own[pid] || !processAlive(pid) {
			continue
		}
		args := line[sp:]
		m := resumeSidRe.FindStringSubmatch(args)
		if m == nil || !strings.EqualFold(m[1], sid) {
			continue
		}
		// 只认 claude 本体（含 ~/.local/bin/claude 之类的包装路径），
		// 避免 grep/编辑器里恰好出现同串 sid 造成误报。
		fields := strings.Fields(args)
		if len(fields) == 0 || filepath.Base(fields[0]) != "claude" {
			continue
		}
		return pid, "终端命令行", true
	}
	return 0, "", false
}

// processAlive 用 0 号信号探活（不发真信号；进程不存在/僵尸归主时返回 false）。
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
