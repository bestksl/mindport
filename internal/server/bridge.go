// fleet bridge（v0.8）：网页与 Claude Code 原生体验之间的桥。
//
// 旧链路（v0.7）每说一句话 spawn 一次 `claude --bg --resume`：会话正忙时
// 官方拒绝并发续跑，话只能落进信箱等一个默认没装的钩子注入，留言经常永久
// 卡死；思考/回答也只有整轮写进 transcript 后才看得见。
//
// bridge 给每支「网页开出去的船队」养一个常驻
//
//	claude -p --input-format stream-json --output-format stream-json \
//	       --include-partial-messages --verbose
//
// 子进程：stdin 是实时消息通道（忙时写入，CLI 自动排队，当前轮结束立刻接着
// 跑，等价于人在终端里抢话）；stdout 是官方事件流——逐 token 的 thinking/
// text delta、工具调用、每轮 result，网页据此逐字直播，不再等整轮落盘。
//
// 兼容：CLI 自己开的会话（TUI/桌面/旧 --bg）没有 bridge 进程，仍走
// transcript 轮询 + 空闲 resume 的老链路；一旦网页对它说话，就为它续一个桥。
package server

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// guard 兜住桥后台 goroutine 的 panic：一支船队的意外绝不能崩掉整个 serve。
func (b *fleetBridge) guard(fp *fleetProc, where string) {
	if x := recover(); x != nil {
		log.Printf("fleet bridge panic sid=%s where=%s: %v", fp.sid, where, x)
		fp.mu.Lock()
		if !fp.killed && fp.state != bridgeDead {
			fp.state = bridgeDead
			fp.errMsg = fmt.Sprintf("桥内部异常（%s）：%v；刷新后可重新开船", where, x)
		}
		fp.mu.Unlock()
		b.publishState(fp)
	}
}

// 桥进程状态。
const (
	bridgeStarting   = "starting"   // 子进程已起，带着首条消息等 system/init
	bridgeCold       = "cold"       // 空船：CLI 收到第一条消息前不发 init，进程待命
	bridgeWaiting    = "waiting"    // 等用户补充/确认（黄灯）
	bridgeWorking    = "working"    // 一轮请求在跑（等同官方 busy）
	bridgeIdle       = "idle"       // 上一轮 result 已回，等下一句
	bridgeRestarting = "restarting" // 中断/掉线收束中，新代 --resume 桥接续
	bridgeDead       = "dead"       // 子进程退出且不再重起（拆船/熔断）
)

// maxAutoRestarts 是桥进程意外死亡时连续自动重起的熔断阈值。
const maxAutoRestarts = 3

// partialBlock 是当前轮还没落进 transcript 的流式内容块。
type partialBlock struct {
	Kind string `json:"kind"` // thinking | text | tool
	Name string `json:"name,omitempty"`
	Text string `json:"text"`
}

// fleetProc 是一支船队的常驻桥。CLI 进程可换代（中断/意外退出后 --resume
// 重起），sid 与本结构不变；gen 标识代数，旧代 goroutine 凭它静默退场。
type fleetProc struct {
	sid   string
	dir   string
	name  string
	cmd   *exec.Cmd
	stdin io.WriteCloser

	wmu sync.Mutex // 串行化 stdin 写入与换代换管

	spawnMu sync.Mutex // 串行化换代：中断重起/掉线自愈/Start 复活不得并发 spawn

	mu       sync.Mutex
	gen      uint64 // 进程代数，每 spawn 一代 +1
	deaths   int    // 连续意外死亡数（成功收到 init 清零）
	killed   bool   // 拆船：本次退出不要重起
	procDone chan struct{}
	state    string
	blocks   []partialBlock
	queued   []string // 忙时注入、尚未跑完的消息（原文，用于页面展示与撤回）
	errMsg   string
	born     time.Time
}

// curGen 读当前代数（旧代 readLoop/waitGen 用它判断自己是否已过期）。
func (fp *fleetProc) curGen() uint64 {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	return fp.gen
}

// curGenState 读当前状态。
func (fp *fleetProc) curGenState() string {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	return fp.state
}

func (fp *fleetProc) snapshotLocked() (state string, blocks []partialBlock, queued []string, errMsg string) {
	state = fp.state
	blocks = append([]partialBlock(nil), fp.blocks...)
	queued = append([]string(nil), fp.queued...)
	return state, blocks, queued, fp.errMsg
}

func (fp *fleetProc) setState(state string) {
	fp.mu.Lock()
	fp.state = state
	fp.mu.Unlock()
}

// fleetBridge 管全部桥进程与事件订阅。
type fleetBridge struct {
	srv *Server

	mu    sync.Mutex
	procs map[string]*fleetProc

	subMu sync.Mutex
	subs  map[chan bridgeEvent]struct{}
}

// bridgeEvent 推给浏览器的实时事件（/api/live SSE）。
type bridgeEvent struct {
	Type   string         `json:"type"` // state | partial | fleet
	SID    string         `json:"sid,omitempty"`
	State  string         `json:"state,omitempty"`
	Blocks []partialBlock `json:"blocks,omitempty"`
	Queued []string       `json:"queued,omitempty"`
	Err    string         `json:"err,omitempty"`
}

func newFleetBridge(s *Server) *fleetBridge {
	return &fleetBridge{srv: s, procs: map[string]*fleetProc{}, subs: map[chan bridgeEvent]struct{}{}}
}

func (b *fleetBridge) subscribe() chan bridgeEvent {
	ch := make(chan bridgeEvent, 64)
	if b == nil {
		return ch // 没有桥（测试/精简装配）：事件永远不来，chan 仅占位
	}
	b.subMu.Lock()
	b.subs[ch] = struct{}{}
	b.subMu.Unlock()
	return ch
}

func (b *fleetBridge) unsubscribe(ch chan bridgeEvent) {
	if b == nil {
		return
	}
	b.subMu.Lock()
	delete(b.subs, ch)
	b.subMu.Unlock()
}

func (b *fleetBridge) publish(ev bridgeEvent) {
	b.subMu.Lock()
	defer b.subMu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default: // 慢客户端丢中间态，state/partial 每秒若干帧，丢得起
		}
	}
}

func (b *fleetBridge) publishState(fp *fleetProc) {
	fp.mu.Lock()
	state, blocks, queued, errMsg := fp.snapshotLocked()
	fp.mu.Unlock()
	ev := bridgeEvent{Type: "state", SID: fp.sid, State: state, Blocks: blocks, Queued: queued, Err: errMsg}
	b.publish(ev)
	b.publish(bridgeEvent{Type: "fleet"}) // 船队列表/角标也要跟着翻灯
}

// stateOf 返回某 sid 的桥状态；没有桥（CLI 会话/离线）返回 ok=false。
func (b *fleetBridge) stateOf(sid string) (state string, alive bool) {
	if b == nil {
		return "", false
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		return "", false
	}
	fp.mu.Lock()
	defer fp.mu.Unlock()
	return fp.state, fp.state != bridgeDead
}

// dirOf/nameOf 取桥进程登记的项目目录与船名。
func (b *fleetBridge) dirOf(sid string) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		return ""
	}
	return fp.dir
}

func (b *fleetBridge) nameOf(sid string) string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		return ""
	}
	return fp.name
}

// snapshot 给船台 stream 端点带上当前流式块与队列。
func (b *fleetBridge) snapshot(sid string) (state string, blocks []partialBlock, queued []string, alive bool) {
	if b == nil {
		return "", nil, nil, false
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		return "", nil, nil, false
	}
	fp.mu.Lock()
	defer fp.mu.Unlock()
	state, blocks, queued, _ = fp.snapshotLocked()
	return state, blocks, queued, fp.state != bridgeDead
}

// Start 为一支船队起桥。resumeSID 非空=续旧会话；为空=新开（sid 由我们
// 生成、--session-id 指定，立刻可绑定/跳转）。起桥后送入第一句话。
func (b *fleetBridge) Start(dir, resumeSID, name, firstMsg string) (string, *fleetProc, error) {
	if b == nil {
		return "", nil, fmt.Errorf("即时桥未启用")
	}
	exe := b.srv.claudeExe()
	if exe == "" {
		return "", nil, fmt.Errorf("本机找不到 claude CLI")
	}
	sid := resumeSID
	if sid == "" {
		u, err := newUUID()
		if err != nil {
			return "", nil, err
		}
		sid = u
	}
	b.mu.Lock()
	fp := b.procs[sid]
	if fp != nil {
		fp.mu.Lock()
		alive := fp.state != bridgeDead && !fp.killed
		fp.mu.Unlock()
		b.mu.Unlock()
		if alive {
			return sid, fp, nil // 已有活桥，不重复起
		}
	} else {
		fp = &fleetProc{sid: sid, dir: dir, name: name, born: time.Now()}
		b.procs[sid] = fp
		b.mu.Unlock()
	}
	if err := b.spawn(fp, resumeSID != "", name, firstMsg); err != nil {
		return sid, fp, err
	}
	return sid, fp, nil
}

// spawn 启动 fp 的新一代 CLI 进程（第一代由 Start 调；中断/掉线后的换代也走
// 这里）。resume=true 用 --resume <sid>，false 用 --session-id <sid>。
// firstMsg 非空时点火并等 init；空船置 cold 即返回。
//
// CLI 2.1.310 实测：result 后进程常驻可接下一句；但 control_request/interrupt
// 会让 -p 进程立刻退出（无回执）——所以「中断」=旧代收束后 spawn 新一代。
func (b *fleetBridge) spawn(fp *fleetProc, resume bool, name, firstMsg string) error {
	// 换代全程持锁（含最长 20s init 等待）：中断重起、掉线自愈、Start 复活
	// 可能同时发生，两代 CLI 绝不能并存同写 transcript。
	fp.spawnMu.Lock()
	defer fp.spawnMu.Unlock()

	// 先收束上一代：正常换代时它已退出（done 已关）；竞态下若还活着，杀掉再走。
	fp.mu.Lock()
	oldProc := fp.cmd
	oldDone := fp.procDone
	killed := fp.killed
	fp.mu.Unlock()
	if killed {
		return fmt.Errorf("船队已拆，不再重起")
	}
	if oldProc != nil && oldDone != nil {
		select {
		case <-oldDone:
		default:
			if oldProc.Process != nil {
				_ = oldProc.Process.Kill()
			}
			select {
			case <-oldDone:
			case <-time.After(2 * time.Second):
				return fmt.Errorf("旧桥进程收不掉，先不换代")
			}
		}
	}

	exe := b.srv.claudeExe()
	if exe == "" {
		return fmt.Errorf("本机找不到 claude CLI")
	}
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--include-partial-messages",
		"--verbose",
		"--permission-mode", "acceptEdits",
	}
	if resume {
		args = append(args, "--resume", fp.sid)
	} else {
		args = append(args, "--session-id", fp.sid)
	}
	if name != "" {
		args = append(args, "-n", name)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = fp.dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrBuf := &limitedBuffer{n: 4000}
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("claude 没接活：%w", err)
	}

	done := make(chan struct{})
	fp.wmu.Lock()
	fp.mu.Lock()
	fp.gen++
	gen := fp.gen
	fp.killed = false
	fp.cmd = cmd
	fp.stdin = stdin
	fp.procDone = done
	fp.state = bridgeStarting
	fp.blocks = nil
	fp.errMsg = ""
	fp.mu.Unlock()
	fp.wmu.Unlock()

	go b.readLoop(fp, stdout, gen)
	go b.waitGen(fp, cmd, stderrBuf, gen, done)

	// CLI 在 stream-json 下收到第一条 user 消息前不会吐 system/init（实测
	// 2.1.310：空 stdin 只发 hook/title 事件）。所以首条消息必须先写进 stdin，
	// 再等 init；不开口的空船则标 cold 直接返回，第一句 Say 时 CLI 才点火。
	if firstMsg == "" {
		fp.mu.Lock()
		if fp.gen == gen && fp.state == bridgeStarting {
			fp.state = bridgeCold
		}
		fp.mu.Unlock()
		b.publishState(fp)
		return nil
	}
	if err := b.writeUser(fp, firstMsg); err != nil {
		return err
	}

	initOK := false
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		fp.mu.Lock()
		st := fp.state
		errMsg := fp.errMsg
		curGen := fp.gen
		fp.mu.Unlock()
		if curGen != gen {
			return fmt.Errorf("桥进程已被新一代接替")
		}
		if errMsg != "" {
			return fmt.Errorf("桥进程启动失败：%s", clipText(errMsg, 300))
		}
		if st != bridgeStarting {
			initOK = true
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !initOK {
		_ = cmd.Process.Kill()
		return fmt.Errorf("桥进程 20s 没报 init（CLI 登录态/网络异常？）")
	}
	fp.mu.Lock()
	fp.deaths = 0 // 跑起来了：意外死亡计数清零
	fp.mu.Unlock()
	return nil
}

// writeUser 把一句话按 stream-json 协议写进桥的 stdin（底层，不做排队判断）。
func (b *fleetBridge) writeUser(fp *fleetProc, msg string) error {
	payload, _ := json.Marshal(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": msg,
		},
	})
	payload = append(payload, '\n')
	fp.wmu.Lock()
	_, err := fp.stdin.Write(payload)
	fp.wmu.Unlock()
	if err != nil {
		return fmt.Errorf("消息没送进会话：%w", err)
	}
	return nil
}

// Say 向船队注入一句话。桥正忙时 CLI 自动排队（等价终端抢话），返回 queued=true。
func (b *fleetBridge) Say(sid, msg string) (queued bool, err error) {
	if b == nil {
		return false, fmt.Errorf("即时桥未启用")
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		return false, fmt.Errorf("这支船队没有桥")
	}
	// 中断收束/掉线重起中：等新一代就位（spawn 空船是毫秒级，给 8s 兜底）。
	if st := fp.curGenState(); st == bridgeRestarting || st == bridgeStarting {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			st = fp.curGenState()
			if st != bridgeRestarting && st != bridgeStarting {
				break
			}
		}
	}
	fp.mu.Lock()
	if fp.state == bridgeDead {
		fp.mu.Unlock()
		return false, fmt.Errorf("桥进程已退出")
	}
	// working/starting/waiting 写入由 CLI 自己排队续跑；cold/idle 直接开跑。
	isWorking := fp.state == bridgeWorking || fp.state == bridgeStarting || fp.state == bridgeWaiting
	if isWorking {
		fp.queued = append(fp.queued, msg)
	}
	fp.mu.Unlock()

	if err := b.writeUser(fp, msg); err != nil {
		return false, err
	}
	return isWorking, nil
}

// Interrupt 中断当前轮（网页版 Esc）。CLI 2.1.310 实测：-p 进程收到
// control_request/interrupt 会立即退出（无回执、rc=1），所以中断=旧代收束+
// --resume 起新一代，网页只见同一 sid 短暂 restarting。
// cancelQueued=true 连排队的话一起撤；false 时排队话由新桥原样重投（CLI
// 内部队列随进程蒸发，且这些话尚未落 transcript，不重投就丢了）。
func (b *fleetBridge) Interrupt(sid string, cancelQueued bool) error {
	if b == nil {
		return fmt.Errorf("即时桥未启用")
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		return fmt.Errorf("这支船队没有桥")
	}
	fp.mu.Lock()
	if fp.killed || fp.state == bridgeDead {
		fp.mu.Unlock()
		return fmt.Errorf("船队已经拆了")
	}
	fp.gen++ // 提前宣告旧代作废：旧 waitGen/readLoop 凭代数不等静默退场，不会误判「意外死亡」重复重起
	oldDone := fp.procDone
	oldProc := fp.cmd
	pending := append([]string(nil), fp.queued...)
	if cancelQueued {
		fp.queued = nil
		pending = nil
	}
	fp.blocks = nil
	fp.state = bridgeRestarting
	fp.errMsg = ""
	fp.mu.Unlock()
	b.publishState(fp)

	// 先礼后兵：协议中断 → 等旧代真退出 → 兜底 SIGKILL。
	// 绝不能让两代 CLI 同时写同一份 transcript。
	payload, _ := json.Marshal(map[string]any{
		"type":          "control_request",
		"action":        "interrupt",
		"cancel_queued": cancelQueued,
	})
	payload = append(payload, '\n')
	// 此刻 spawn 尚未开始，stdin 必是旧代管道；wmu 只防与并发 Say 写交错。
	fp.wmu.Lock()
	_, _ = fp.stdin.Write(payload)
	fp.wmu.Unlock()
	if oldDone != nil {
		select {
		case <-oldDone:
		case <-time.After(1500 * time.Millisecond):
			if oldProc != nil && oldProc.Process != nil {
				_ = oldProc.Process.Kill()
			}
			select {
			case <-oldDone:
			case <-time.After(1500 * time.Millisecond):
				return fmt.Errorf("旧桥进程收不掉，先不重起，稍后重试")
			}
		}
	}

	// 新代 --resume 接续；有排队话则拿第一句点火，其余写进新 CLI 的队列。
	first := ""
	if len(pending) > 0 {
		first = pending[0]
	}
	if err := b.spawn(fp, true, "", first); err != nil {
		return fmt.Errorf("中断了但新桥没接上：%w", err)
	}
	b.reinject(fp, pending)
	return nil
}

// reinject 把排队消息按序写进新桥：pending[0] 已用于 spawn 点火，
// pending[1:] 写进新 CLI 的队列（忙时自动排队续跑）。
func (b *fleetBridge) reinject(fp *fleetProc, pending []string) {
	for i := 1; i < len(pending); i++ {
		if err := b.writeUser(fp, pending[i]); err != nil {
			break
		}
	}
}

// Kill 硬停桥进程（拆船）：标记 killed，waitGen 见标记不再重起。
func (b *fleetBridge) Kill(sid string) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		return false
	}
	fp.mu.Lock()
	if fp.killed || fp.state == bridgeDead {
		fp.mu.Unlock()
		return false
	}
	fp.killed = true
	fp.state = bridgeDead
	fp.blocks = nil
	fp.queued = nil
	proc := fp.cmd
	fp.mu.Unlock()
	if proc != nil && proc.Process != nil {
		_ = proc.Process.Kill()
	}
	b.publishState(fp)
	return true
}

// waitGen 收束一代进程：拆船→dead；换代→静默；意外退出→有界自动 --resume。
func (b *fleetBridge) waitGen(fp *fleetProc, cmd *exec.Cmd, stderrBuf *limitedBuffer, gen uint64, done chan struct{}) {
	defer b.guard(fp, "wait")
	err := cmd.Wait()
	close(done)

	fp.mu.Lock()
	if gen != fp.gen {
		fp.mu.Unlock()
		return // 新一代已就位，旧代静默退场
	}
	if fp.killed {
		fp.state = bridgeDead
		fp.mu.Unlock()
		b.publishState(fp)
		return
	}
	// 意外退出（CLI 崩溃/被杀/网络断绝）：标记重起中，退避后自动 --resume。
	fp.deaths++
	deaths := fp.deaths
	fp.state = bridgeRestarting
	fp.blocks = nil
	if err != nil && fp.errMsg == "" {
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			fp.errMsg = s
		}
	}
	fp.mu.Unlock()
	b.publishState(fp)

	if deaths > maxAutoRestarts {
		fp.mu.Lock()
		fp.state = bridgeDead
		if fp.errMsg == "" {
			fp.errMsg = "桥进程反复退出，已停止自动重连；刷新页面可重新开船"
		}
		fp.mu.Unlock()
		b.publishState(fp)
		return
	}
	time.Sleep(time.Duration(deaths) * 500 * time.Millisecond)

	// 与中断同理：CLI 内部队列随进程蒸发，网页队列里的话没落过 transcript，
	// 由新一代按序重投（第一句点火，其余进新 CLI 队列）。
	fp.mu.Lock()
	pending := append([]string(nil), fp.queued...)
	fp.mu.Unlock()
	first := ""
	if len(pending) > 0 {
		first = pending[0]
	}
	if rerr := b.spawn(fp, true, "", first); rerr != nil {
		fp.mu.Lock()
		fp.state = bridgeDead
		fp.errMsg = rerr.Error()
		fp.mu.Unlock()
		b.publishState(fp)
		return
	}
	b.reinject(fp, pending)
}

// readLoop 逐行解析 CLI 事件流，维护状态与流式块。
// gen 是本代代数：换代后旧 goroutine 不得再写状态，直接退出。
func (b *fleetBridge) readLoop(fp *fleetProc, r io.Reader, gen uint64) {
	defer b.guard(fp, "read")
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // init 行带全量工具表，给 4MB 上限
	for sc.Scan() {
		if fp.curGen() != gen {
			return // 已换代，旧事件流作废
		}
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev struct {
			Type      string          `json:"type"`
			Subtype   string          `json:"subtype"`
			Status    string          `json:"status"`
			SessionID string          `json:"session_id"`
			IsError   bool            `json:"is_error"`
			Event     json.RawMessage `json:"event"`
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "system":
			// init=会话起来了；status=requesting=一轮新请求开始（含排队话开场）。
			switch ev.Subtype {
			case "init":
				fp.mu.Lock()
				if fp.state == bridgeStarting || fp.state == bridgeCold {
					fp.state = bridgeIdle
				}
				fp.errMsg = ""
				fp.mu.Unlock()
				b.publishState(fp)
			case "status":
				fp.mu.Lock()
				switch {
				case ev.Status == "requesting":
					fp.state = bridgeWorking
					fp.blocks = nil
					fp.errMsg = ""
				case strings.Contains(ev.Status, "wait"):
					// waiting_for_input 等：黄灯，别再当工作中
					fp.state = bridgeWaiting
				}
				fp.mu.Unlock()
				b.publishState(fp)
			}
		case "stream_event":
			if b.applyStreamEvent(fp, ev.Event) {
				b.publishState(fp)
			}
		case "assistant":
			// 完整消息落盘前后到达：什么都不做，transcript 轮询会拿到权威行。
		case "result":
			fp.mu.Lock()
			fp.state = bridgeIdle
			fp.blocks = nil
			if len(fp.queued) > 0 {
				fp.queued = fp.queued[1:] // 跑完一句，弹出队首
			}
			if ev.IsError {
				fp.errMsg = "这一轮报错了，看下文详情"
			}
			fp.mu.Unlock()
			b.publishState(fp)
		}
	}
}

// applyStreamEvent 把 API 增量事件折叠成流式块；返回是否有可见变化。
func (b *fleetBridge) applyStreamEvent(fp *fleetProc, raw json.RawMessage) bool {
	var ev struct {
		Type         string `json:"type"`
		Index        int    `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content_block"`
		Delta struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
			Text     string `json:"text"`
			Partial  string `json:"partial_json"`
		} `json:"delta"`
	}
	if json.Unmarshal(raw, &ev) != nil {
		return false
	}
	fp.mu.Lock()
	defer fp.mu.Unlock()
	changed := false
	switch ev.Type {
	case "content_block_start":
		kind := ""
		switch ev.ContentBlock.Type {
		case "thinking":
			kind = "thinking"
		case "text":
			kind = "text"
		case "tool_use":
			kind = "tool"
		}
		if kind != "" {
			fp.blocks = append(fp.blocks, partialBlock{Kind: kind, Name: ev.ContentBlock.Name})
			changed = true
		}
	case "content_block_delta":
		if ev.Index < 0 || ev.Index >= len(fp.blocks) {
			return false
		}
		blk := &fp.blocks[ev.Index]
		switch ev.Delta.Type {
		case "thinking_delta":
			blk.Text += ev.Delta.Thinking
			changed = true
		case "text_delta":
			blk.Text += ev.Delta.Text
			changed = true
		case "input_json_delta":
			blk.Text += ev.Delta.Partial // 工具入参只攒着，页面默认不展开
		}
	case "message_stop":
		// 不清空：权威 transcript 行到船台有秒级轮询延迟，留到 result 再清，
		// 避免回答在气泡里闪一下消失。
	}
	return changed
}

// ---------- helpers ----------

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// bridgeStatus 把桥状态翻成官方三态词（busy|idle）；死桥返回 ok=false。
func (b *fleetBridge) bridgeStatus(sid string) (status string, online bool) {
	state, alive := b.stateOf(sid)
	if !alive {
		return "", false
	}
	switch state {
	case bridgeWorking, bridgeStarting:
		return "busy", true
	case bridgeWaiting:
		return "waiting", true
	case bridgeRestarting:
		return "restarting", true
	default:
		return "idle", true
	}
}

// enrichRows 用桥的实时状态/船名覆盖扫盘行，并补入「桥已起、transcript
// 还没落盘、发现口也没刷出」的全新船队。archived 名册行打归档标。
func (s *Server) enrichRows(rows []dirRow, home string) []dirRow {
	if s.bridge == nil {
		return rows
	}
	fleets := s.V.Fleets()
	hidden := s.V.HiddenSIDs()
	kept := rows[:0]
	for i := range rows {
		if hidden[rows[i].SID] {
			continue // 已拆船：从思港抹掉
		}
		kept = append(kept, rows[i])
	}
	rows = kept
	have := map[string]bool{}
	for i := range rows {
		row := &rows[i]
		have[row.SID] = true
		if status, online := s.bridge.bridgeStatus(row.SID); online {
			row.Online = true
			row.Status = status
			fs := fleetStateOf(true, status)
			row.Class = "is-" + fs.Key
			row.StateText = fs.Text
		}
	}
	// 名册里的船名（-n「短码-需求」）优先于首句标题。
	bySID := map[string]int{}
	for i := range rows {
		bySID[rows[i].SID] = i
	}
	for _, f := range fleets {
		if f.Name == "" {
			continue
		}
		if i, ok := bySID[f.SID]; ok {
			rows[i].Name = f.Name
		}
	}
	// 桥里有、列表没有（全新会话 transcript 未生）的船队补行。
	s.bridge.mu.Lock()
	var fresh []*fleetProc
	for _, fp := range s.bridge.procs {
		if !have[fp.sid] {
			fresh = append(fresh, fp)
		}
	}
	s.bridge.mu.Unlock()
	for _, fp := range fresh {
		if hidden[fp.sid] {
			continue
		}
		fp.mu.Lock()
		state := fp.state
		fp.mu.Unlock()
		if state == bridgeDead {
			continue
		}
		status := "busy"
		switch state {
		case bridgeIdle, bridgeCold:
			status = "idle"
		case bridgeWaiting:
			status = "waiting"
		case bridgeRestarting:
			status = "restarting"
		}
		name := fp.name
		if fi, ok := s.V.Fleet(fp.sid); ok && fi.Name != "" {
			name = fi.Name
		}
		row := dirRow{
			Online: true, Status: status, Name: name,
			Proj:     filepath.Base(fp.dir),
			ShowProj: true, HideCount: true,
		}
		row.SID = fp.sid
		row.Dir = fp.dir
		row.Title = name
		fs := fleetStateOf(true, status)
		row.Class = "is-" + fs.Key
		row.StateText = fs.Text
		rows = append(rows, row)
	}
	return rows
}

// limitedBuffer 是给桥进程 stderr 留的有上限缓冲。
type limitedBuffer struct {
	buf []byte
	n   int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if len(l.buf) < l.n {
		end := len(p)
		if room := l.n - len(l.buf); end > room {
			end = room
		}
		l.buf = append(l.buf, p[:end]...)
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string { return string(l.buf) }
