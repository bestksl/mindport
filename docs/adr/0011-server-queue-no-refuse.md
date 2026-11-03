# ADR-0011：永不拒话——服务端权威排队与席位留言

日期：2026-11-03
状态：已接受
关联：[ADR-0008](0008-session-transcript-bridge.md)（常驻桥）、[ADR-0009](0009-fleet-is-session.md)、
`t3-parity-plan.md`（对标 t3code 的 queue/run 语义）

## 背景

两条旧行为会让用户的话「发不出去」：

1. 会话正被终端 TUI / 桌面客户端占着同一 sid 时，网页起桥直接报 409
   「会话在别处开着」，用户必须自己去那边退出。
2. 忙时插话走 stdin 直写，CLI 内部排队——消息一旦写入就不可撤回、不可编辑，
   「中断并清空」是唯一的管理手段。

t3code 的核心原则是**用户输入永远被收下**：agent 进程归服务端所有，浏览器
只是事件订阅者；排队项是服务端实体，可 cancel/edit/promote。

## 决策

### 1. 任何入口不拒绝人话

- 席位被外部 claude 客户端占着：话落 `.port/state/mail/<sid>.jsonl`，
  接口返回 `delivered=held`；4 秒一跳的 held watcher 探席位，空出后
  `--resume` 起桥、首条点火、其余原子认领（PullMail）投递。
- serve 重启后 `resumeHeldWatches` 自动把有未投递留言的会话重新挂上 watcher。
- 网页、手机、多观众之间没有任何 per-client 锁。

### 2. 排队消息归服务端，不提前交给 CLI

- 忙/起跑/换代中收到的消息只追加到 `fleetProc.queued`，一轮 `result`
  收束后由 `pumpQueue` 投队头开下一轮；消息未投递前可逐条撤回/编辑/插队。
- 插队（promote / Send now）= `interrupt` 换代，该句独占点火，其余排队保留。
- 崩溃/换代时队列不丢：waitGen 重起用队头点火，spawn 失败把队头放回。
- 例外（死锁规避）：`waiting`（AskUserQuestion 等人回答）时发话就是**答案**，
  必须立刻直写 stdin 放行当前轮；进队列会等到一个永远不会来的 result。
- HTTP：`POST /sessions/{sid}/queue/{remove,edit,promote}`，返回最新队列快照。

### 3. 停止三级与强制结束

协议 cancel（1.5s）→ SIGKILL 兜底（1.5s）→ `force=1` 跳过协议直接收进程。
前端「⏳ 停止中 n 秒」8 秒无响应露出「⚡ 强制结束」。队列随队列参数处置。

## 后果

- 队列只在桥进程内存（held/mail 在盘上）：serve 在队列非空时崩溃会丢
  已 Say 但未跑的忙时插话——P1 用 runs 表持久化彻底解决（见对标计划）。
- 排队期间编辑/撤回有了明确语义（消息未交给 agent）；promote 后该句不可撤。
- 状态徽标细化为 待审批 / 待输入 / 工作中 / 连接中 / 空闲·可续跑 / 已停止。
