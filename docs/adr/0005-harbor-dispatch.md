# ADR-0005：调度港模型——锚地、命令式派船、归港硬门禁

日期：2026-10-26
状态：已接受（v0.4 实现）
取代：ADR-0004 中「publish 可任领、assignee 只是建议」的部分

## 背景

v0.3 的模型是「人发布、agent 任领」：发布后进公共 ready 池，任何执行者按重要程度
自取；assignee 只表达期望，不构成权限；网页带着身份切换器，发布者本人也能领取、
归港。实际使用中用户（调度长官本人）给出了明确定性：

> 我是强制性的指派。我的角色是港口的调度长官和客户，需求是集装箱；
> CC 和 Codex 是负责解决问题的船，出去就是干活，回来就要有结果。

由此暴露三个问题：

1. **指派没有强制力**：派给 A 的货 B 也能领，「命令」形同建议；
2. **权责不清**：调度台（网页）能代船做所有操作，人会误触，且无法追责；
3. **归港没有门槛**：船可以空口归港，验收项没完成也能结束，客户无法验货。

## 决策

### 1. 新增 anchored（锚地）状态

```
draft ──publish(无 assignee)──▶ anchored ──assign(点名)──▶ ready
  │                                                          │
  └────────publish(带 assignee)──────────────────────────────┘
                                    ready ──claim(仅承担船)──▶ in_progress
                                                                  │
                                          blocked ◀──block(仅承担船)
                                              │  └──unblock──┘
                                              └──assign(受阻改派)──▶ ready
                                                                  │
                                              in_progress/blocked ──done
```

- **anchored**：货已备齐（打磨完验收标准）但还没决定派给谁。锚地对船不可见：
  不进任何船的 `port next`，不能被 claim。
- **publish 语义收窄**：草稿放行——不带 `--to` 进锚地，带 `--to` 直接派船。
  CLI 的 `port new` 默认也进锚地（v0.3 是直接 ready），网页建任务一律 draft。
- 旧版「ready 无 assignee」的存量任务（任领池遗留）在读取时自动迁移为 anchored；
  带 assignee 的 ready 保持待接令。迁移只在 canonicalize 中做一次状态修正。

### 2. 派船是命令：assignee 必填且唯一可接令

- 新命令 `port dispatch T### --to <ship>`（store: `Assign`）：锚地派船、
  待接令改派、受阻换船（货回码头重新待接令）；in_progress 在海上不能改派。
- claim 在锁内校验 `assignee == by`，不符返回 `ErrNotAssignee`；
  `port next [--by]` 只返回派给该船的 ready 任务。
- 每次派船/改派写署名日志（dispatched / reassigned / 原船受阻）。

### 3. 调度台权责：网页只能调度与验货

- 移除网页的身份切换 cookie 与 claim/done/block/unblock 路由及按钮；
  HTTP 端不再存在这些操作入口（旧路径返回 405/404）。
- 网页固定以调度长官署名（`jmzx@studio`），只保留：放行（publish）、
  派船/改派（assign）、验货勾选（check）、批注（log）、草稿/锚地编辑。
- 船的动作只能从船自己的 CLI 发出（身份由 `PORT_ACTOR` / `--by` 声明，
  与 ADR-0004 的心跳身份一致；单机私用，不引入鉴权）。
- **派船即封舱**：非 draft/anchored 状态禁止编辑任务单，验收标准以开船时为准；
  调整走批注或改派。

### 4. 归港硬门禁 + 强制归港逃生门

`Done(id, by, result, force, reason)`：

- 有验收项：必须全部勾选；
- 无验收项：必须提供 `--result` 交付说明（作为日志条目留痕）；
- 不满足时返回 `ErrResultRequired`；
- 逃生门：`--force --reason`，reason 必填，归港日志中永久记录
  「⚠️ 强制归港（船名），理由：…」，防止真死锁，也防止滥用无痕。

### 5. 在线心跳改由守护循环保活（同期修正）

ADR-0004 的钩子只在 SessionStart/UserPromptSubmit 瞬间写一次心跳（TTL 2 分钟），
一轮 agent 工作常超过 2 分钟，Stop 又立即 away——实测看板上几乎看不到在线灯。
v0.4 新增 `port heartbeat`：钩子调用后 fork 脱离会话的后台循环，每 45 秒
`pgrep -x claude` 探活：在就用 `PresentKeepalive` 续期（保留 task/note），
CLI 退出后自动 away 并清理 pidfile。**不引入 launchd/cron**（用户已否决系统托管）：
守护随会话自生自灭，pidfile 防重复，新会话可接管旧守护。

## 后果

- 看板从三列变四列：锚地待派 / 待接令 / 出港中 / 已归港；手机端保持单列纵排。
- agent 提示词与 ADR-0004 时代不同：不再「自取最重要的 ready」，
  而是「只听派给自己的令」（见 docs/agent-flow.md）。
- 取消状态（cancelled）仍未进入 UI；调度长官撤回锚地货目前靠直接改文件，
  后续如需高频使用再补 `port cancel`。
- 船名单前以常量花名册（claude@studio / codex@studio）+ 心跳外船补充渲染；
  船队扩大时再配置化。
