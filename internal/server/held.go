// held watch：会话被外部 claude 客户端（终端 TUI / 桌面 / 另一个守护）占着
// 席位时，网页不报错、不丢话（对标 t3：用户输入永远被收下，只有队列）。
//
// 消息先落 mail 文件：装了 mail-pull 钩子的终端下一轮提交就会带走；
// watcher 每 4 秒探一次席位，外部客户端一退出就把留言原子认领（PullMail，
// 与钩子互斥、不会重复注入）并 --resume 起桥投递。
package server

import (
	"context"
	"errors"
	"log"
	"time"
)

// heldProbeInterval：席位探测节奏（externalHolder 自身带 4s 超时）。
var heldProbeInterval = 4 * time.Second

// holdWatch 为 sid 确保一个「等席位空出」watcher（幂等：已在等则不重复起）。
func (b *fleetBridge) holdWatch(sid, dir string) {
	b.heldMu.Lock()
	defer b.heldMu.Unlock()
	if _, ok := b.held[sid]; ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.held[sid] = cancel
	go b.heldLoop(sid, dir, ctx)
}

// stopHeld 停掉 sid 的 watcher（桥接管 / 已投递 / 会话删除）。
func (b *fleetBridge) stopHeld(sid string) {
	b.heldMu.Lock()
	defer b.heldMu.Unlock()
	if cancel, ok := b.held[sid]; ok {
		cancel()
		delete(b.held, sid)
	}
}

// heldWatching 报告 sid 是否正在等席位（测试/状态用）。
func (b *fleetBridge) heldWatching(sid string) bool {
	b.heldMu.Lock()
	defer b.heldMu.Unlock()
	_, ok := b.held[sid]
	return ok
}

func (b *fleetBridge) heldLoop(sid, dir string, ctx context.Context) {
	t := time.NewTicker(heldProbeInterval)
	defer t.Stop()
	for {
		// 立刻探一次：发消息时外部客户端可能刚好已经退出。
		if done := b.heldTick(sid, dir); done {
			b.stopHeld(sid)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// heldTick 推进一步。返回 true 表示 watcher 可以收工。
func (b *fleetBridge) heldTick(sid, dir string) bool {
	// 1) 我们的桥已经在跑（另一条路先起了）：把留言认领出来直接注进桥。
	if st, alive := b.stateOf(sid); alive && st != bridgeDead && st != bridgeRestarting {
		b.deliverMailIntoBridge(sid)
		return true
	}
	// 2) 没有待投递留言（被 mail-pull 钩子拉走，或用户撤了）：收工。
	if n, _ := b.srv.V.MailPending(sid); n == 0 {
		return true
	}
	// 3) 席位还被外部客户端占着：继续等。
	if _, _, held := b.externalHolder(sid); held {
		return false
	}
	// 4) 席位空出：先起桥（Start 内部会再探一次席位，竞态时返回
	//    errSessionHeldElsewhere，留言保持未认领、下轮再试），成功后再原子
	//    认领留言，与 mail-pull 钩子互斥去重。
	pending, err := b.srv.V.MailPending(sid)
	if err != nil || pending == 0 {
		return true
	}
	first, _ := b.srv.V.FirstMail(sid)
	if first == "" {
		return true
	}
	if err := b.startWithFirst(dir, sid, first); err != nil {
		if errors.Is(err, errSessionHeldElsewhere) {
			return false
		}
		log.Printf("held watch: 起桥投递失败 sid=%s: %v（留言保留，下次发消息再试）", sid, err)
		return true
	}
	b.deliverMailIntoBridge(sid)
	return true
}

// startWithFirst 起一座桥并用 first 点火（errBridgeWarming 也算成功：
// 话已在子进程 stdin 里）。测试可替换。
func (b *fleetBridge) startWithFirst(dir, sid, first string) error {
	if b.testStart != nil {
		return b.testStart(dir, sid, first)
	}
	_, _, err := b.Start(dir, sid, "", first)
	if errors.Is(err, errBridgeWarming) {
		return nil
	}
	return err
}

// deliverMailIntoBridge 原子认领全部未投递留言，首条已用于起桥点火，
// 其余逐条写进桥 stdin（CLI 自动排队续跑）；若桥已不在则什么都不做。
func (b *fleetBridge) deliverMailIntoBridge(sid string) {
	mails, err := b.srv.V.PullMail(sid)
	if err != nil || len(mails) == 0 {
		return
	}
	b.mu.Lock()
	fp := b.procs[sid]
	b.mu.Unlock()
	if fp == nil {
		// 桥在认领与此刻之间消失：留言已回标，只能等下一次发消息时重排。
		log.Printf("held watch: 认领留言后桥消失 sid=%s，%d 条待人工重发", sid, len(mails))
		return
	}
	for _, m := range mails[1:] {
		fp.mu.Lock()
		if fp.state == bridgeWorking || fp.state == bridgeStarting || fp.state == bridgeWaiting {
			fp.queued = append(fp.queued, m.Text)
		}
		fp.mu.Unlock()
		if err := b.writeUser(fp, m.Text); err != nil {
			log.Printf("held watch: 追加留言写入失败 sid=%s: %v", sid, err)
			return
		}
	}
	b.publishState(fp)
}

// resumeHeldWatches serve 启动时把「有未投递留言、且没有桥」的会话重新挂
// 上 watcher：进程重启不影响排队承诺。
func (b *fleetBridge) resumeHeldWatches() {
	if b == nil || b.srv == nil {
		return
	}
	for _, f := range b.srv.V.Fleets() {
		if f.SID == "" || f.Dir == "" {
			continue
		}
		n, err := b.srv.V.MailPending(f.SID)
		if err != nil || n == 0 {
			continue
		}
		if _, alive := b.stateOf(f.SID); alive {
			continue
		}
		b.holdWatch(f.SID, f.Dir)
	}
}
