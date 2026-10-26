# Agent 工作流：Claude Code / Codex 如何与思港协作

思港是一个**调度港**（v0.4 模型）：

- **人是调度长官 + 客户**：把念头澄清成集装箱（任务）、打磨验收标准、
  **下命令派船**。网页端是纯调度台——只放行、派船/改派、验货、批注，
  **不能代船接令或归港**。
- **Claude / Codex 是船**：只看得到**派给自己**的货；接令（claim）、
  汇报（log/check/block）、归港（done）都只能在船自己的 CLI 完成。
- **派船是命令，不是建议**：一票货派给某艘船后，只有它能接令，
  没有公共任务池、不能抢单。

## 状态流

```
draft（草稿·备货中）
   │  publish（放行）
   ▼
anchored（锚地待派，船看不到）
   │  dispatch/assign（派船，必须点名）
   ▼
ready（已派船 · 待该船接令）
   │  claim（只有被指派的船）
   ▼
in_progress（出港中）
   │  block（船上报受阻，必须给原因）
   ▼
blocked（受阻：调度长官可批注，或改派别的船）
   │
   ▼
done（归港：验收全勾，或附交付结果；网页验货）
```

## 调度长官（人）：从念头到派船

网页建任务一律先进**草稿（draft）**。在详情页补好「验收标准」（完成定义）后：

- **放进锚地**：货已备齐但还没决定派谁；锚地里的货船在 CLI 完全看不到；
- **直接派船**：选 Claude / Codex（带在线状态），一步到「待接令」。

锚地卡片/详情页随时可**派船**；待接令的货可**改派**；船受阻上浮后，
货回码头，可**受阻改派**给别的船（全程留痕）。

派船即**封舱**：草稿/锚地阶段可随意编辑标题、正文、验收标准；
一旦派船，任务单冻结，要调整只能批注或改派——验收标准以开船时为准。

CLI 同义命令：

```bash
port new "..."                       # 默认进锚地
port new "..." --draft               # 先存草稿打磨
port publish T0001                   # 草稿放行 → 锚地
port publish T0001 --to claude@studio  # 草稿直接派船（任意会话可接）
port dispatch T0001 --to codex@studio  # 锚地派船 / 改派 / 受阻换船
# 指定船位：只在该项目目录开会话的船能接令（防止别的项目会话接错票）
port dispatch T0001 --to claude@studio --berth /Users/jmzx/studio/jiepai-app
```

**船位（berth）**：同一艘船可能在多个项目目录里同时开着会话。网页派船时可在下拉里
选「claude · 📍jiepai-app · 在线」这样的具体会话；任务记下 `assign_cwd` 后，
`port next`/`port claim` 只在对应项目目录生效，别处看到的提示是「这票货派给了别的船位」。
不指定船位（「任意会话」）则该船在哪个目录都能接。

下拉里每个选项是一个**具体会话**，带官方会话简介（如「三个联动项目」）、
📍目录与运行态（工作中/在线/上次报到 x），也包括已离线的历史会话
（灰显「上次报到 x（新开即接）」）：派到离线会话所在目录的货停在「待接令」，
等该船下次在那个项目目录开会话时自然接到。还可以选
「🆕 在新项目里新开会话接令」，填入新项目的完整目录路径——
货同样先停在待接令，人去那个目录开船即可。网页只负责下命令，不替人开船。

注意派船闸门按**项目目录**匹配（同一会话简介下可能有多个相关任务/念头，
它们在同一目录的会话里都能接），不是绑定某个 session id——会话关了重开不影响接令。

## 一次完整协作（船 / agent 侧）

```bash
# 0.（可选）声明身份，一次会话设置一次
export PORT_ACTOR=claude@studio      # codex@studio / 你的船名

# 1. 听令：只列出「派给我、待我接令」的货（锚地货与别人的货都看不到）
port next                            # 默认按当前工作目录过滤船位
port next --json                     # 给程序用
port next --by claude@studio         # 也可显式指定船名
port next --cwd /path/to/project     # 显式指定船位（默认取当前目录）

# 2. 读令：接令前先看完整上下文
port show T0002                      # 描述 + 验收标准 + 历史日志
port claim T0002                     # 只有被派船的你能接令；接令即心跳
                                     # 网页上立即显示 claude 在线、正在做 T0002

# 3. 干活：进度只有两种「有意义的单位」——叙事日志、验收勾选
port log T0002 "int8 基准跑完，单帧 27.4ms，满足预算"
port check T0002 1                   # 勾选第 1 条验收标准
port block T0002 "等 Vs3 真机实测数据"   # 卡住立刻上浮，必须给原因
port unblock T0002                   # 调度长官批注/补给后继续（也只有你能操作）

# 4. 归港（硬门禁，见下）
port done T0002                                   # 有验收项且已全勾
port done T0002 --result "替代方案已验证，数据见日志"  # 没有验收项时必须交结果
port done T0002 --force --reason "验收项已失效，客户口头确认"  # 逃生门
```

## 归港硬门禁

船回来必须带结果，store 层强制：

- 任务写了验收项：**必须全部勾选**才能归港；
- 没写验收项：归港必须附 `--result` 交付说明；
- 特殊情况（死锁、验收项失效）：`--force --reason` 可强制归港，
  **理由永久写入日志**，调度台验货时看得到。

归港后调度长官在网页对照验收标准**验货**（勾选/批注都是留痕）；
有问题在批注里指出，船下一票货会看到。

## 在线心跳（网页顶栏可见）

船位的「在/不在」分两层：

- **点卯（钩子）**：SessionStart / UserPromptSubmit 钩子调
  `port heartbeat`（stdin JSON 带 `session_id`/`cwd`），按「船 + 项目会话」
  写一条心跳文件 `.port/state/agents/<actor>/<session>.json`，
  顶栏显示 📍项目名与会话简介；claim/done 自动维护任务号。
  **v0.5 起 heartbeat 只点卯，不再 fork 后台探活守护**。
- **生死判定（官方发现口）**：serve 每 30 秒跑一次
  [`claude agents --json`](https://code.claude.com/docs/en/agent-view)——
  这是官方文档明确提供给 scheduler 等外部程序的会话状态接口，输出是**活会话全集**，
  含 `sessionId/cwd/name`（会话简介）/`kind`/`state`(working/blocked/...)。
  在全集里=在线，缺席=已退出（心跳文件保留，灰显为离线船位）。
  发现口不可用时自动退回 2 分钟 TTL 判定，远程 ssh 船继续走扁平心跳。

同船如果同时存在会话级心跳与旧版扁平心跳（`<actor>.json`，手动 `port present`），
看板会**折叠扁平记录**；纯扁平客户端（远程船，actor 名不同）不受影响。

Codex 等没有钩子的 agent，可在项目提示里要求开工先 `port present`、
收工 `port away`。用户级 `~/.claude/settings.json` 装上同样的钩子后，
**任何目录**的 Claude Code 会话都会自动点卯。

## 设计原则

1. **命令式派船，不是公共任务池**：调度长点名叫船，`next` 只听自己的令；
   非承担船的 claim 直接被 store 拒绝（`ErrNotAssignee`），网页根本没有这个按钮。
   派船还能点名到**船位**（具体项目目录的会话）：街拍项目的会话接不到闲鱼的票，
   错目录 claim 会得到 `ErrWrongBerth`。
2. **目标导向，不是进度导向**：没有截止日期、没有紧急度、没有完成百分比。
   任务只有重要程度（`critical/high/normal/someday`）与所服务的目标（`goal`）。
   派船只问「什么对目标最重要、哪艘船合适」，不问「什么快逾期了」。
3. **进度 = 验收清单 + 叙事日志**，不是心跳、不是百分比：
   - `port check` 勾选「完成定义」中的具体条目；
   - `port log` 记录过程，调度台实时（SSE）看得到发生了什么。
4. **权责清晰，代操不可能**：接令/归港/受阻只属于船且只能在 CLI 发生；
   派船/改派/验货/批注只属于调度长官且主要在网页。每条日志都带署名。
5. **JSON 是给 agent 的一等输出**：`list/show/next` 全部支持 `--json`，
   正文与元数据分离——agent 同步状态只碰 frontmatter，永不解析正文。

## HTTP API（daemon 已在跑时）

调度台的网页入口；agent 优先用 CLI。

| 方法与路径 | 作用 |
|---|---|
| `GET /api/next?actor=claude@studio&cwd=/repo/x` | 派给该船、且船位匹配（cwd 为空或等于 assign_cwd）的最重要任务 |
| `GET /api/tasks?status=anchored` | 任务列表（`draft/anchored/ready/in_progress/blocked/done`） |
| `GET /api/agents` | 各船位心跳列表（每条含在线标志、session_id、dir、任务号、状态行） |
| `POST /tasks/{id}/publish` | 放行草稿（表单 `assignee` 空=进锚地；`actor`=直接派船任意会话；`actor|/项目目录`=指定船位） |
| `POST /tasks/{id}/assign` | 派船/改派/受阻换船（表单 `assignee` 取值同上，船位可具体到在线会话） |
| `POST /tasks/{id}/check` `/log` | 验货勾选、调度批注 |

注意：HTTP 端**没有** claim/done/block/unblock 路由——这些动作只在船的 CLI。

CLI 与网页走的是同一个 store：**无论哪边写入，另一边 SSE 实时刷新**。

## 给 agent 的提示词模板（可贴进 CLAUDE.md / 系统提示）

```
本项目任务通过思港 MindPort 调度（CLI: port，vault: ~/MindPort）。
你是船，只接调度长官派给你的货：开工前 `port next` 听令（只显示派给你、
且船位匹配当前项目目录的货），`port show T###` 读上下文，
`port claim T###` 接令后再动手。
每完成一个有意义的步骤 `port log`；满足一条验收标准就 `port check T### n`。
归港是硬门禁：验收项全勾，或 `port done --result "交付说明"`；
特殊情况 `port done --force --reason "..."`，理由永久留痕。
遇到阻塞 `port block T### "原因"`，不要静默卡住。
```
