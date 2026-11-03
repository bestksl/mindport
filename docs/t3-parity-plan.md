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
| 忙时插话 | stdin 直写 CLI 自动排队；UI「发送=停下当前轮换跑这句（豆包式），Ctrl+Enter=排队」；ReplaceTurn 中断换代 | bridge_actions.go, session_templates.go |
| 排队展示 | 显示已排队句数 +「中断并清空」（仅全撤，不能逐条改/删） | session_templates.go renderQueue |
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

## 实施批次（调研后细化）

- **P0（今夜）**：占用改排队不报错；对话页状态/按钮/队列管理向 t3 看齐；
  Waiting 子态（待审批/待输入/计划就绪）；停止/中断语义校准。
- **P1**：thread 收纳架（Working/Snoozed/Settled 对应工作中/稍后/已了结）、
  排队消息逐条编辑/删除/立即发送、断线补发游标、Goal 体验对齐。
- **P2**：子代理嵌套展示、审批 Always allow 粒度、桌面端细节。
