# T3 Code 对标计划：核心机制搬过来，港口皮肤套上去

> 2026-11-03 晚启动。用户要求：把 t3code（MIT 许可，本地克隆见调研记录）
> 经过多人维护的核心机制整体对标，对话体验做到一致；思港继续是 Go 单二进制 +
> 原生 HTML/JS，不引入 Node/React 构建，视觉上仍是像素海港皮肤。

## 对标原则

1. **搬机制，不搬框架**：t3 是 TS/React/RN/Node monorepo；思港是 Go + net/http +
   go:embed。机制（状态机、队列语义、事件协议、UX 规则）用 Go 重实现，
   1:1 对齐行为与文案语义，像素港口皮肤不动。
2. **英文协议层向 t3 靠拢**：对前端暴露的状态名/事件名尽量与 t3 同义
   （working/waiting/idle/queued…），降低未来继续对标的摩擦；
   中文 UI 用用户熟悉的话。
3. **保留思港差异点**：任务状态机（draft→待办→待领取→进行中→受阻→完成）、
   目标（对标 t3 Goal）、与终端 TUI **共享同一批 claude 会话**（t3 的 thread
   全由 server 独占，思港刻意兼容 TUI/桌面客户端自己开的会话）。

## 现状盘点（思港已有的底子，不少机制已接近 t3）

| 维度 | 思港现状 | 文件 |
|---|---|---|
| agent 驱动 | 每会话常驻 `claude -p --input-format stream-json --output-format stream-json --include-partial-messages --verbose`，stdin 写消息、stdout 事件流 | bridge.go |
| 浏览器实时通道 | 全局 SSE `/api/live`（state/partial/fleet/perm 事件）+ 会话 JSON 轮询 1.5s（页面隐藏暂停）+ transcript 字节偏移增量 | bridge_actions.go, session.go |
| 桥状态机 | starting/cold/waiting/working/idle/restarting/dead，连续 3 次意外死亡熔断 | bridge.go |
| 忙时插话 | **服务端权威 fp.queued**：忙/起跑/换代中消息只存内存，轮间 pumpQueue 投递；UI「发送=停下当前轮换跑这句（豆包式），Ctrl+Enter=排队」；ReplaceTurn/Promote 中断换代 | bridge.go, bridge_actions.go |
| 排队展示 | 虚线卡逐条 ⏩立即发送/✏️取回编辑/✕撤回，>3 折叠；另有「中断并清空」 | session_templates.go renderQueue |
| 审批 | 只读工具自动放行；危险工具 MCP 回调挂起 → 手机卡片 Approve/Deny + Auto/Plan/无人值守三模式 | perm.go |
| 直播渲染 | thinking/text/tool/result partial 块，GFM 逐字渲染，权威 transcript 行到达后顶替 | bridge.go, session_templates.go |
| 多端看同一会话 | SSE 扇出给所有页面；只读查看不互斥 | bridge.go subs |
| 终端占用 | 起桥前探测同 sid 的外部 claude 客户端（agents 发现口 + ps 兜底），**冲突时直接 409 报错** | bridge_guard.go |
| 留言兜底 | 桥起不来 → `.port/state/mail/<sid>.jsonl`，UserPromptSubmit(mail-pull) 钩子下一轮注入；页面有「待终端接收」气泡 | store/mail.go |
| 会话分组 | 🌊 工作中 / 🟡 待你输入 / ⚓ 空闲 / 🏁 已完成 / 🕒 历史 | dir_templates.go |
| 结局标记 | ✅ 圆满结束 / ⏹ 中途停止（人标的 mark，覆盖状态文案） | session.go |

## 已知 P0 缺口（用户直接感知）

1. **「会话在别处开着」硬报错**：t3 从不拒绝用户输入（queue/steer 永远收）。
   目标行为：终端占着同 sid 时不再 409——消息进等待队列，页面显示
   「终端开着，已排队；那边一退出/下一轮自动送达」，可撤回；服务端探到
   席位空出自动 --resume 起桥投递。
2. 其余 P0/P1/P2 清单待三路调研（传输层 / 对话页 UX / run 队列与多观众）
   回来后填本表。

## t3 机制调研结论（2026-11-03，文件:行号以本地克隆为准）

克隆：`/Users/jmzx/.claude/jobs/2456a6fc/tmp/t3code`。核心在
`apps/server/src/orchestration-v2/`（O2）、`packages/contracts/src/orchestrationV2.ts`
（契约）、`packages/client-runtime/src/`（前端状态机）。

### A. 永拒话：服务器拥有 agent，观众无锁

- agent 子进程归**服务器**所有（ClaudeAdapterV2 持 `claude-agent-sdk` query；
  第三方 agent 走 ACP `claude --acp` stdio JSON-RPC）。浏览器连接只是事件订阅者，
  断开不杀 run。**全仓没有任何 per-client 占用/ writer 锁**；并发安全靠
  每线程一把命令串行锁 `threadDispatch.withLock`（Orchestrator.ts:13635）+
  写时乐观版本号（EventSink.ts:95-126）。
- 浏览器↔服务器：单条 WebSocket `/ws`（effect/RPC 三帧：request/response/update），
  HTTP 只做登录、首屏快照、历史分页；WS 鉴权用一次性 ticket
  （POST /api/auth/websocket-ticket），5s ping/15s 判死。
- 事件溯源：`orchestration_v2_events`（全局自增 sequence），事务**提交后**才
  PubSub 扇出；每连接带 `afterSequence` 独立游标，缺口 ≤128 事件且 ≤8MiB
  重放真实事件，否则整体快照替换（ThreadStream.ts:18；ws.ts:828-1033）。
  命令带客户端生成的 commandId，`command_receipts` 表幂等去重。

### B. run/队列状态机（契约 orchestrationV2.ts:590-602）

- RunStatus 十态：`preparing/queued/starting/running/waiting/completed/
  interrupted/failed/cancelled/rolled_back`；active=前四态，终态=后五态，
  **queued 既非 active 也非 terminal**。每线程至多一个 active run。
- 排队不是"一句话"：建队时就预创 run+attempt+rootNode+message 全套行，
  `status=queued` + `queuePosition`。服务端排序（QueuedRunOrder.ts：自动消息优先，
  再 queuePosition/ordinal）；排队 run 支持 cancel/edit/reorder/start/promote-to-steer。
- `queueHeld`：Stop、worktree 没就绪、用量受限、启动失败、重启后——队列被按住；
  只被三类事件释放：**真实用户新消息**、`queue.resume`、无人值守自动释放
  （Orchestrator.ts:5874-5898, 13233-13296）。停住之后用户发一句话 = 续跑。
- 两段式完成：provider turn completed 先落 **waiting**（checkpoint 未 durable），
  `checkpoint.capture` effect 成功才 completed（RunExecutionService.ts:714；
  CheckpointCaptureService.ts:197）。
- steer（中途改向）≠ interrupt：能 active steer 就注高优先级消息、不取消当前
  turn；不支持就 interrupt + 同 run 新 attempt（reason=steering_restart）。
- stop 三级：协议取消通知（10s）→ close session（10s）→ 只杀本线程独占的
  进程组；共享进程绝不杀。全程写 run_interrupt_request/result turn item。
- 审批/问询发生在 **running 期间**（不是 run 态）：RuntimeRequest
  kind=command/file-read/file-change/mcp-elicitation/permission/user_input…，
  status=pending/resolved/expired/cancelled，**无服务端超时**；turn 中断时
  未决请求统一 cancel。Plan ready = interactionMode=plan 且有 active
  proposed_plan（ProjectionStore.ts:1586）。
- Goal 是 provider thread 上的轻量字段（objective + active/paused/blocked/
  usage_limited/budget_limited/complete + token/checks 计数），不是独立状态机；
  /goal 维护命令插队位置 1 并停挡路 run。
- subagent（delegate_task）：父线程建 subagent 节点+独立子线程，父 run 保持
  running 靠 activeChild* 计数门控收尾，中断级联终止子代理。

### C. 与思港现状的机制映射

| t3 机制 | 思港现状 | 差距/处置 |
|---|---|---|
| 服务器拥有 agent，viewer 无锁 | 网页桥本就服务器拥有；但刻意兼容 TUI/桌面共享 sid，靠 seat 探测 | **已补 held 队列**（永不 409，watch 席位空出自动续） |
| queued run 持久化+可编辑/取消/重排 | 桥内存 fp.queued（仅全撤）+ held/mail 文件队列 | 逐条取消/编辑/立即发送；队列落盘随架构轨做 |
| queueHeld：Stop 按住队列，用户发话释放 | Stop=esc 换代，Ctrl+Enter 排队 | 语义校准：停止后排队话保持 hold，下句话释放 |
| afterSequence 重放/快照 | 1.5s JSON 轮询 + transcript 字节偏移 + SSE 状态扇出 | 架构轨：单 WS + 序号游标 |
| 协议级取消三级 stop | control_request/interrupt + 等 procDone + SIGKILL 兜底 | 已等价三级；补「无响应」可强杀的 UI 态 |
| waiting=checkpoint 窗口；审批/问询是 running 期请求 | busy/waiting 二态 + reconcileBusy 把 AskUser 翻 waiting | 补 Waiting 子态：待审批/待输入/计划就绪 |
| 命令幂等 commandId 收据 | 表单 POST 无幂等键 | 架构轨补 |
| ACP `claude --acp` | `claude -p stream-json` 常驻桥 | 架构轨评估切换（审批/取消更标准） |

## 实施批次

### P0（2026-11-03 夜，体验对齐，不动骨架）

1. ✅ 占用改排队不报错（held watcher + mail 落盘 + 前端 held 气泡，已提交）。
2. ✅ 对话页状态徽标向 t3 看齐：工作中 / **待审批**（琥珀，perms 优先）/
   **待输入**（靛蓝，waiting_for 有具体问题）/ 待你输入 / **连接中…** /
   **空闲 · 可续跑** / 已停止。列表行同步区分「空闲（在线可续跑）」与「已停止」，
   restarting 文案统一为连接中。
   - 待补：**计划就绪**（plan mode proposed_plan）需识别 transcript 里的
     ExitPlanCode/Plan 工具调用，挪 P1 与 runs 状态一起做。
3. ✅ 排队消息逐条管理（服务端权威 fp.queued，消息不再提前写进 CLI stdin）：
   每条虚线卡可 ✕ 撤回 / ✏️ 取回编辑（Edit in the composer）/ ⏩ 立即发送
   （promote-to-steer：中断当前轮、该句点火、其余排队保留）；>3 条折叠；
   端点 `POST /sessions/{sid}/queue/{remove,edit,promote}`。
   - 关键语义：waiting（AskUserQuestion）时发话 = 答案，直写 stdin 放行，
     绝不进队列（否则 result 永不到达 = 死锁）；有测试锁定。
   - restarting 入队与 cold pumpQueue 竞态：Say 补踢 +500ms kickQueuedAfter。
4. ✅ Stop：「⏳ 停止中 n s」计时；8 秒无响应露出「⚡ 强制结束」，
   服务端 `force=1` 跳过协议取消直接 SIGKILL 旧代再 --resume（t3 Force stop）。
5. ✅ 多端同看：撤回/编辑/promote 都走 publishState，SSE 90ms 合并轮询即时刷新。

### P1（架构轨，多夜，搬 t3 核心框架）

- 单 WebSocket `/ws` + 极简 JSON 帧；ticket 升级鉴权；5s ping/指数退避重连。
- 事件表 + 全局 sequence：先 commit 后发布；订阅三帧 snapshot/event/synchronized；
  afterSequence 重放，>128 或 >8MiB 退化快照；命令 commandId 收据幂等。
- runs 表即队列：queued/starting/running/waiting/completed/interrupted/failed/
  cancelled 八态 + queue_position/queue_held；每线程串行归约 + 乐观守卫；
  Stop 置 held、真实用户消息释放；重启恢复五规则。
- 两段式 waiting→completed（outbox worker 替代桥进程内存回调）。
- thread 收纳架 Working/Snoozed/Settled；Goal 轻量字段对齐。

### P2（能力补全）

- 评估切 `claude --acp`（标准化审批/取消/plan update）；steer active 模式。
- 子代理嵌套卡片、审批 always-allow 粒度（CLI 会话权限 vs 自管 DB 二选一）。
- checkpoint/rolled_back、worktree 延迟启动、provider 进程池跨会话共享。
