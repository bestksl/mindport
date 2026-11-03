# Agent 工作流：Claude Code / Codex 如何与思港协作

思港是一个**任务台**（当前协作模型）：

- **你是指派者 + 验收人**：把念头澄清成任务、完善验收标准、
  **定向指派**。网页端是纯任务台——只发布、指派/改派、验收、批注，
  **不能代助手领取或完成**。
- **Claude / Codex 是助手**：只看得到**指派给自己**的任务；领取（claim）、
  汇报（log/check/block）、完成（done）都只能在它自己的 CLI 完成。
- **指派是命令，不是建议**：一个任务指派给某位助手后，只有它能领取，
  没有公共任务池，别人看不到也领不走。

视觉皮肤保留港口主题（灯塔、货轮、配色都在），但业务说法统一为
任务/助手/工作目录/目标，命令名不变。

## 状态流

```
draft（草稿·编辑中）
   │  publish（发布到待办区）
   ▼
anchored（待办，其他助手看不到）
   │  dispatch/assign（指派，必须指定助手）
   ▼
ready（已指派 · 等待领取）
   │  claim（只有被指派的助手）
   ▼
in_progress（进行中）
   │  block（助手上报受阻，必须给原因）
   ▼
blocked（受阻：你可批注，或重新指派给别的助手）
   │
   ▼
done（已完成：验收全勾，或附交付结果；网页验收）
```

## 指派者（人）：从念头到指派

网页建任务一律先进**草稿（draft）**。在详情页补好「验收标准」（完成定义）后：

- **放进待办区**：任务已完善好但还没决定派给谁；待办区里的任务助手在 CLI 完全看不到；
- **直接指派**：选 Claude / Codex（带在线状态），一步到「待领取」。

待办区卡片/详情页随时可**指派**；待领取的任务可**重新指派**；助手受阻上浮后，
任务停住，可**受阻重新指派**给别的助手（全程留痕）。

指派即**锁定**：草稿/待办阶段可随意编辑标题、正文、验收标准；
一旦指派，任务单冻结，要调整只能批注或重新指派——验收标准以指派时为准。

CLI 同义命令：

```bash
port new "..."                       # 默认进待办区
port new "..." --draft               # 先存草稿
port publish T0001                   # 草稿发布 → 待办区
port publish T0001 --to claude@studio  # 草稿直接指派（任意会话可领）
port dispatch T0001 --to codex@studio  # 待办区指派 / 改派 / 受阻换助手
# 指定工作目录（--berth）：只在该项目目录开会话的助手能领取
# （防止别的项目会话领错任务）
port dispatch T0001 --to claude@studio --berth /Users/jmzx/studio/jiepai-app
```

**工作目录（berth）**：同一位助手可能在多个项目目录里同时开着会话。网页指派时可在下拉里
选「claude · 📍jiepai-app · 在线」这样的具体会话；任务记下 `assign_cwd` 后，
`port next`/`port claim` 只在对应项目目录生效，别处看到的提示是「这个任务指派给了别的工作目录」。
不指定工作目录（「任意会话」）则该助手在哪个目录都能领。

下拉里每个选项是一个**具体会话**，带官方会话简介（如「三个联动项目」）、
📍目录与运行态（工作中/在线/上次在线时间），也包括已离线的历史会话
（灰显「上次在线 x（新开即接）」）：派到离线会话所在目录的任务停在「待领取」，
等该助手下次在那个项目目录开会话时自然接到。还可以选
「🆕 在新项目里新开会话来做」，填入新项目的完整目录路径——
任务同样先停在待领取，你去那个目录开一个 Claude Code 会话即可。网页只负责下命令，不替你开会话。

注意指派闸门按**项目目录**匹配（同一会话简介下可能有多个相关任务/念头，
它们在同一目录的会话里都能接），不是绑定某个 session id——会话关了重开不影响领取。

## 一次完整协作（助手 / agent 侧）

```bash
# 0.（可选）声明身份，一次会话设置一次
export PORT_ACTOR=claude@studio      # codex@studio / 你的助手名

# 1. 领任务：只列出「指派给我、待我领取」的任务（待办区与别人的任务都看不到）
port next                            # 默认按当前工作目录过滤
port next --json                     # 给程序用
port next --by claude@studio         # 也可显式指定助手名
port next --cwd /path/to/project     # 显式指定工作目录（默认取当前目录）

# 2. 读任务：领取前先看完整上下文
port show T0002                      # 描述 + 验收标准 + 历史日志
port claim T0002                     # 只有被指派的你能领取；领取即报在线
                                     # 网页上立即显示 claude 在线、正在做 T0002

# 3. 干活：进度只有两种「有意义的单位」——叙事日志、验收勾选
port log T0002 "int8 基准跑完，单帧 27.4ms，满足预算"
port check T0002 1                   # 勾选第 1 条验收标准
port block T0002 "等 Vs3 真机实测数据"   # 卡住立刻上浮，必须给原因
port unblock T0002                   # 你批注/补充后继续（也只有你能操作）

# 4. 完成（硬门禁，见下）
port done T0002                                   # 有验收项且已全勾
port done T0002 --result "替代方案已验证，数据见日志"  # 没有验收项时必须交结果
port done T0002 --force --reason "验收项已失效，客户口头确认"  # 逃生门
```

## 完成硬门禁

助手完成任务必须带结果，store 层强制：

- 任务写了验收项：**必须全部勾选**才能完成；
- 没写验收项：完成必须附 `--result` 交付说明；
- 特殊情况（死锁、验收项失效）：`--force --reason` 可强制完成，
  **理由永久写入日志**，任务台验收时看得到。

完成后你在网页对照验收标准**验收**（勾选/批注都是留痕）；
有问题在批注里指出，助手做下一个任务时会看到。

## 在线状态（网页顶栏可见）

助手是否在线分两层：

- **报在线（钩子）**：SessionStart / UserPromptSubmit 钩子调
  `port heartbeat`（stdin JSON 带 `session_id`/`cwd`），按「助手 + 项目会话」
  写一条状态文件 `.port/state/agents/<actor>/<session>.json`，
  顶栏显示 📍项目名与会话简介；claim/done 自动维护任务号。
  **v0.5 起 heartbeat 只报在线，不再 fork 后台探活守护**。
- **生死判定（官方发现口）**：serve 每 30 秒跑一次
  [`claude agents --json`](https://code.claude.com/docs/en/agent-view)——
  这是官方文档明确提供给 scheduler 等外部程序的会话状态接口，输出是**活会话全集**，
  含 `sessionId/cwd/name`（会话简介）/`kind`/`state`(working/blocked/...)。
  在全集里=在线，缺席=已退出（状态文件保留，灰显为离线会话）。
  发现口不可用时自动退回 2 分钟 TTL 判定，远程 ssh 会话继续走扁平心跳。

同一位助手如果同时存在会话级心跳与旧版扁平心跳（`<actor>.json`，手动 `port present`），
看板会**折叠扁平记录**；纯扁平客户端（远程会话，actor 名不同）不受影响。

Codex 等没有钩子的 agent，可在项目提示里要求开工先 `port present`、
收工 `port away`。用户级 `~/.claude/settings.json` 装上同样的钩子后，
**任何目录**的 Claude Code 会话都会自动报在线。

## 设计原则

1. **命令式指派，不是公共任务池**：你指定谁，`next` 只看自己的任务；
   非承担者的 claim 直接被 store 拒绝（`ErrNotAssignee`），网页根本没有这个按钮。
   指派还能指定到**工作目录**（具体项目目录的会话）：街拍项目的会话领不到闲鱼的任务，
   错目录 claim 会得到 `ErrWrongBerth`。
2. **目标导向，不是进度导向**：没有截止日期、没有紧急度、没有完成百分比。
   任务只有重要程度（`critical/high/normal/someday`）与所服务的目标（`goal`）。
   指派只问「什么对目标最重要、哪位助手合适」，不问「什么快逾期了」。
3. **进度 = 验收清单 + 叙事日志**，不是在线状态、不是百分比：
   - `port check` 勾选「完成定义」中的具体条目；
   - `port log` 记录过程，任务台实时（SSE）看得到发生了什么。
4. **权责清晰，代操不可能**：领取/完成/受阻只属于助手且只能在 CLI 发生；
   指派/改派/验收/批注只属于你且主要在网页。每条日志都带署名。
5. **JSON 是给 agent 的一等输出**：`list/show/next` 全部支持 `--json`，
   正文与元数据分离——agent 同步状态只碰 frontmatter，永不解析正文。

## HTTP API（daemon 已在跑时）

任务台的网页入口；agent 优先用 CLI。

| 方法与路径 | 作用 |
|---|---|
| `GET /api/next?actor=claude@studio&cwd=/repo/x` | 指派给该助手、且工作目录匹配（cwd 为空或等于 assign_cwd）的最重要任务 |
| `GET /api/tasks?status=anchored` | 任务列表（`draft/anchored/ready/in_progress/blocked/done`） |
| `GET /api/agents` | 各会话在线状态列表（每条含在线标志、session_id、dir、任务号、状态行） |
| `POST /tasks/{id}/publish` | 发布草稿（表单 `assignee` 空=进待办区；`actor`=直接指派任意会话；`actor|/项目目录`=指定工作目录） |
| `POST /tasks/{id}/assign` | 指派/改派/受阻换助手（表单 `assignee` 取值同上，可具体到在线会话） |
| `POST /tasks/{id}/check` `/log` | 验收勾选、批注 |

注意：HTTP 端**没有** claim/done/block/unblock 路由——这些动作只在助手的 CLI。

CLI 与网页走的是同一个 store：**无论哪边写入，另一边 SSE 实时刷新**。

## 给 agent 的提示词模板（可贴进 CLAUDE.md / 系统提示）

```
本项目任务通过思港 MindPort 管理（CLI: port，vault: ~/MindPort）。
你是助手，只领取指派给你的任务：开工前 `port next` 看待领任务（只显示指派给你、
且工作目录匹配当前项目目录的任务），`port show T###` 读上下文，
`port claim T###` 领取后再动手。
每完成一个有意义的步骤 `port log`；满足一条验收标准就 `port check T### n`。
完成是硬门禁：验收项全勾，或 `port done --result "交付说明"`；
特殊情况 `port done --force --reason "..."`，理由永久留痕。
遇到阻塞 `port block T### "原因"`，不要静默卡住。
```
