package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParsePSOutput(t *testing.T) {
	// 真实 `ps -axo pid=,tty=,lstart=,args=` 行样（lstart 无时区段）。
	raw := "" +
		"12345 ttys001 Sun Oct 26 21:03:11 2026 /Users/jmzx/.local/bin/claude\n" +
		// 单位数日号带空格，Fields 会拍平；守护进程无终端
		"23456 ?? Wed Oct  7 09:00:00 2026 /x/claude --bg-pty-host /tmp/cc-daemon-501/x/pty/s.sock\n" +
		// 桌面 app 托管会话：--agent + --session-id，参数很多
		"34567 ?? Tue Oct 28 08:30:00 2026 /Users/jmzx/.local/share/claude/versions/2.1.310/claude --session-id 11111111-1111-4111-8111-111111111111 --agent claude --inherit-permission-mode auto\n" +
		// 中文 LC_TIME 形态（「日 10/25 …」只有 4 段）：解析不出必须跳过，
		// 实际发现靠 scanProcessSessions 强制 LC_TIME=C 调 ps。
		"4429 ttys000 日 10/25 17:46:48 2026 claude\n" +
		"garbage line\n"

	got := parsePSOutput([]byte(raw))
	if len(got) != 3 {
		t.Fatalf("解析出 %d 条，期望 3（坏行应跳过）", len(got))
	}
	if got[0].PID != 12345 || got[0].TTY != "ttys001" {
		t.Errorf("第一条 pid/tty 错误：%+v", got[0])
	}
	if y, mo, d := got[0].Start.Date(); y != 2026 || mo != time.October || d != 26 {
		t.Errorf("启动时间解析错误：%v", got[0].Start)
	}
	if h, mi := got[0].Start.Hour(), got[0].Start.Minute(); h != 21 || mi != 3 {
		t.Errorf("启动时分错误：%v", got[0].Start)
	}
	// 单位数日号（" 7"）也必须解析成功——这是旧版 time.UnixDate 静默全挂的回归点。
	if got[1].Start.Day() != 7 {
		t.Errorf("单位数日号解析错误：%v", got[1].Start)
	}
	if !filepath.IsAbs(got[0].Args) || filepath.Base(splitFirst(got[0].Args)) != "claude" {
		t.Errorf("args 归段错误：%q", got[0].Args)
	}
}

func splitFirst(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			return s[:i]
		}
	}
	return s
}

func TestClassifyProc(t *testing.T) {
	const uuid = "11111111-1111-4111-8111-111111111111"
	tests := []struct {
		name string
		c    procCandidate
		sid  string
		ok   bool
	}{
		{
			name: "桌面 app 托管会话",
			c: procCandidate{
				TTY:  "??",
				Args: "/x/claude --session-id " + uuid + " --agent claude --inherit-permission-mode auto",
			},
			sid: uuid, ok: true,
		},
		{
			name: "pty 陪跑进程带 sid 但不是 --agent",
			c: procCandidate{
				TTY:  "??",
				Args: "/x/claude --bg-pty-host /tmp/s.sock --session-id " + uuid,
			},
			ok: false,
		},
		{
			name: "交互 TUI 裸 claude 占终端",
			c: procCandidate{
				TTY:  "ttys002",
				Args: "/Users/jmzx/.local/bin/claude",
			},
			sid: "", ok: true,
		},
		{
			name: "裸 claude 但无终端（守护化）不算",
			c: procCandidate{
				TTY:  "??",
				Args: "/Users/jmzx/.local/bin/claude",
			},
			ok: false,
		},
		{
			name: "带参数的 claude 命令（agents/一次性）不算 TUI",
			c: procCandidate{
				TTY:  "??",
				Args: "claude agents --json",
			},
			ok: false,
		},
		{
			name: "bg-spare 陪跑（占 tty 但带子命令参数）不算 TUI",
			c: procCandidate{
				TTY:  "ttys011",
				Args: "claude bg-spare --bg-spare /tmp/cc-daemon-501/x/spare/y.claim.sock",
			},
			ok: false,
		},
		{
			name: "daemon run 带 spawned-by JSON 不算 TUI",
			c: procCandidate{
				TTY:  "??",
				Args: `/Users/jmzx/.local/bin/claude daemon run --origin transient --spawned-by {"label":"claude","cwd":"/x","pid":4429}`,
			},
			ok: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, ok := classifyProc(tt.c)
			if ok != tt.ok || sid != tt.sid {
				t.Errorf("classifyProc() = (%q, %v)，期望 (%q, %v)", sid, ok, tt.sid, tt.ok)
			}
		})
	}
}

// writeTranscript 在 home 下造一个某 cwd 的 transcript 文件并把 mtime 设成 now。
func writeTranscript(t *testing.T, home, cwd, sid, firstUser string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "projects", encodeProject(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, sid+".jsonl")
	line := `{"type":"user","timestamp":"2026-10-28T10:00:00.000Z","cwd":"` + cwd +
		`","message":{"role":"user","content":[{"type":"text","text":"` + firstUser + `"}]}}` + "\n"
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
}

// encodeProject 与 transcript.ProjectDir 的编码保持一致（只取编码段）。
func encodeProject(cwd string) string {
	out := make([]byte, 0, len(cwd))
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out = append(out, byte(r))
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

func TestPairProcSessions(t *testing.T) {
	home := t.TempDir()
	const (
		cwdA   = "/Users/me/demo"
		cwdEmp = "/Users/me/empty"
		sidA   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		sidB   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	)
	writeTranscript(t, home, cwdA, sidA, "你好啊")
	// 桌面会话：transcript 在另一个 cwd 下，靠 bySID 直接命中。
	writeTranscript(t, home, "/Users/me/other", sidB, "桌面托管的会话")
	files, err := indexTranscripts(home)
	if err != nil || len(files) != 2 {
		t.Fatalf("indexTranscripts: %v, files=%d", err, len(files))
	}

	now := time.Now()
	pend := []procPending{
		{c: procCandidate{PID: 101, TTY: "ttys001", Start: now}, isTUI: true}, // 配 sidA
		{c: procCandidate{PID: 102, TTY: "ttys002", Start: now}, isTUI: true}, // 同目录第二个 TUI，文件被占→匿名
		{c: procCandidate{PID: 103, TTY: "ttys003", Start: now}, isTUI: true}, // 空目录→匿名
		{c: procCandidate{PID: 201, Start: now}, sid: sidB},                   // 桌面 --agent
	}
	cwds := map[int]string{101: cwdA, 102: cwdA, 103: cwdEmp, 201: "/Users/me/other"}

	got := pairProcSessions(home, pend, cwds, files, map[string]bool{})
	if len(got) != 4 {
		t.Fatalf("配出 %d 条，期望 4", len(got))
	}
	byKey := map[string]cliSession{}
	for _, s := range got {
		byKey[s.SessionID] = s
	}

	a, ok := byKey[sidA]
	if !ok {
		t.Fatalf("TUI1 没配上 transcript：%+v", got)
	}
	if a.Anon || a.CWD != cwdA || a.Name != "你好啊" || a.PID != 101 {
		t.Errorf("TUI1 配对内容错误：%+v", a)
	}
	if a.Status != "waiting" { // 末条 user → 待你输入
		t.Errorf("TUI1 状态错误：%q", a.Status)
	}

	if b := byKey[sidB]; b.Anon || b.CWD != "/Users/me/other" || b.Name != "桌面托管的会话" || b.PID != 201 {
		t.Errorf("桌面会话配对错误：%+v", b)
	}

	anon, ok := byKey["proc-102"]
	if !ok || !anon.Anon || anon.CWD != cwdA || anon.Status != "waiting" {
		t.Errorf("同目录第二个 TUI 应落匿名行：%+v", byKey["proc-102"])
	}
	if a3 := byKey["proc-103"]; !a3.Anon || a3.CWD != cwdEmp {
		t.Errorf("空目录 TUI 应落匿名行带 cwd：%+v", byKey["proc-103"])
	}

	// 排序：真会话在前、匿名垫后。
	if got[len(got)-1].SessionID != "proc-103" || got[len(got)-2].SessionID != "proc-102" {
		t.Errorf("匿名行应垫后，实际顺序：%v", sidList(got))
	}
}

func TestPairProcSessionsTakenAndNoCWD(t *testing.T) {
	home := t.TempDir()
	const (
		cwdA = "/Users/me/demo"
		sidA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		sidK = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	writeTranscript(t, home, cwdA, sidA, "第一句")
	files, _ := indexTranscripts(home)
	now := time.Now()

	// 发现口已占 sidA：该 TUI 不能再配上（避免发现口会话被顶替身份）。
	pend := []procPending{
		{c: procCandidate{PID: 101, TTY: "ttys001", Start: now}, isTUI: true},
		// 桌面会话没有 transcript 且 lsof 没给 cwd：应整条丢弃，不进列表。
		{c: procCandidate{PID: 201, Start: now}, sid: sidK},
	}
	cwds := map[int]string{101: cwdA}
	got := pairProcSessions(home, pend, cwds, files, map[string]bool{sidA: true})
	if len(got) != 1 {
		t.Fatalf("期望 1 条（匿名 TUI），实际 %d: %+v", len(got), got)
	}
	if got[0].SessionID != "proc-101" || !got[0].Anon {
		t.Errorf("被占用的 TUI 应落匿名行：%+v", got[0])
	}
}

func TestAnonSID(t *testing.T) {
	if !isAnonSID("proc-123") || isAnonSID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa") {
		t.Error("isAnonSID 判断错误")
	}
	if anonSID(42) != "proc-42" {
		t.Error("anonSID 格式错误")
	}
}

func sidList(ss []cliSession) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.SessionID
	}
	return out
}
