# 架构决策记录（ADR）

> 注：ADR 是决策时的历史记录，早期条目沿用当时的港口业务隐喻（派船/锚地/船队等）。
> 2026-11-03 起产品文案改用任务/助手/工作目录等主流说法（视觉皮肤不变），
> 术语对照与配对模型见 [ADR-0010](0010-device-pairing-and-plain-words.md)。

| # | 标题 | 状态 | 日期 |
|---|------|------|------|
| [0001](0001-local-first-single-machine.md) | 本地优先 · 单机拓扑与连接方式 | Accepted | 2026-10-25 |
| [0002](0002-language-go.md) | 实现语言选择 Go | Accepted | 2026-10-25 |
| [0003](0003-task-file-format.md) | Vault 结构与文件格式规范 v0（thought/task/note 三类型） | Proposed | 2026-10-25 |
| [0004](0004-presence-and-publish.md) | 草稿/发布模型、执行者在线心跳与会话钩子 | Accepted（派船模型部分被 0005 取代） | 2026-10-25 |
| [0005](0005-harbor-dispatch.md) | 调度港模型：锚地、命令式派船、归港硬门禁 | Accepted | 2026-10-26 |
| [0006](0006-capture-as-draft.md) | 入港即立箱：取消独立闪念/inbox | Accepted | 2026-10-26 |
| [0007](0007-agent-integration.md) | Claude Code 远程/编程化能力的接入边界（架构评估） | Proposed | 2026-10-26 |
| [0008](0008-session-transcript-bridge.md) | 会话船台：transcript 全量桥接与双向回话 | Accepted | 2026-10-26 |
| [0009](0009-fleet-is-session.md) | 船队即会话：三态复用、结局标记、任务挂船队 | Accepted | 2026-10-28 |
| [0010](0010-device-pairing-and-plain-words.md) | 设备配对与大众话术：一次性配对码、设备级会话、去航海化文案 | Accepted | 2026-11-03 |
