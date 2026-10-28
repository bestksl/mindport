# ADR-0009 船队即会话：三态复用、结局标记、任务挂船队

- 状态：Accepted（2026-10-28）
- 接续：[ADR-0008](0008-session-transcript-bridge.md)（会话船台与 transcript 桥接）、[ADR-0005](0005-harbor-dispatch.md)（调度港模型）

## 背景

v0.4~v0.6 把「船队」做成了浮在会话之上的一层聚合视图，又自造了一套调度话术
（调度长官台、君臣父子式的层级称谓）。实际使用中两个问题同时暴露：

1. **概念重复**：船队里装的本来就是当前的 Session，却要两个入口、两套列表；
   用户的心智模型是「一支船队就是一个 Session」。
2. **状态自造**：船台自己拼了一套忙/闲/离线文案，和 `claude agents --json`
   的原生三态（busy / waiting / idle）对不齐，手机上看不出「它在思考」还是
   「轮到我说话」。

另两类数据问题在同期确认：

- Ctrl-C 中断在 transcript 里表现为一条 user 记录（顶层 `interruptedMessageId`，
  内容是 `[Request interrupted by user]` 占位句），旧实现把它当成真人发言，
  渲染成靠右的用户气泡；压缩续接（`isCompactSummary`）同理污染计数与标题。
- 任务与船队之间只有「派船出港」这一条重状态机路径，想把一个想法先**挂**到
  某支船队上跟着走、不触发出港，做不到。

立场也一并明确：思港只是「想法停泊的港口」，港内角色平等，不再使用长官/臣属式措辞。

## 结论

1. **船队 = 会话**：撤掉独立的船队浮层，顶栏只留一个明面入口「船队」→ `/sessions`。
   一支 Session 就是一支船队，会话列表即船队列表。
2. **三态直接复用原生语义**，不另造：
   - `busy` → **工作中**；`waiting` → **待你输入**；
   - 在线 `idle` 与离线同态 → **已停止**（不区分，避免用户做无意义判断）。
3. **会话结局双标记**：人确认会话收束后，手动盖 `✅ 圆满结束` 或 `⏹ 中途停止`；
   标记是独立的 UI 状态，存 `.port/state/session-marks.json`（原子 rename、
   withLock、损坏不阻塞服务）。对该会话再次说话（resume）自动撤结局——
   开口即续，标记不该比内容长寿。
4. **任务是挂在船队上的集装箱**：任务只增加 `Meta.Session` / `Meta.SessionDir`
   两个字段和一条日志，就能「挂」到任意会话；它与 ADR-0005 的派船状态机
   **正交**——挂载不出港、不占船，纯粹是业务想法与一支船队的归属关系。
   底层到此为止，不引入船队-任务的新表或新状态。
5. **中断与压缩是系统状态，不是发言**：transcript 解析期把
   `interruptedMessageId` 记录、新旧两种 `[Request interrupted by user]`
   占位句、`isCompactSummary` 续接统一转为 `role=system` 状态块。
   它们可以在时间线上显示为一枚状态胶囊（✋ 已中断 / 🧵 压缩续接），
   但绝不进用户气泡、不计入消息数、不作为会话标题来源。
6. **去层级措辞**：品牌文案统一为「想法停泊的港口」；仅代码注释与派船工作流
   内部沿用「锚地待派」等既有术语，不对用户呈现长官/臣属称谓。

## 设计要点

- 船台交互对标 CLI：1s 增量轮询（`stream?from=<字节偏移>`，幂等）、思考块默认
  展开 + 打字指示、上翻加载历史；空输入前端 trim 显式拦截，提交体与校验同源
  （URLSearchParams），后端 400 兜底。
- 列表行状态用 class 切换（is-working / is-waiting / is-idle + 离线），
  有结局标记时隐藏实时态、显示结局徽标。
- 挂载在任务详情页用下拉选择会话（按目录分组），看板卡片与船台互相可见归属。
- 解析层语义由 `internal/transcript/system_test.go`、挂载/标记由
  `internal/store/sessions_test.go`、HTTP 面由 `internal/server/fleet_test.go`
  锁定。

## 不做的事

- 不引入船队与任务的多对多关系、层级或权限；一任务最多挂一支船队，可换可摘。
- 不区分「在线空闲」与「离线」的呈现——对用户都是「已停止」。
- 不把中断/压缩块从 transcript 改写或删除，只读时转态。

## 修订注记（v0.7 补丁：发现口补扫、新开船队、锚地可删）

上线后实测两处缺口，在不改本 ADR 结论的前提下补齐：

1. **官方发现口不是全集**：CLI 2.1.310 的 `claude agents --json` 只报 background
   会话；桌面 app 托管会话与交互 TUI（裸 `claude` 主进程）都缺席，船队角标会少算
   （用户眼前开着 7 支，发现口只报 4 支）。补发现只读本机可见状态，不碰 daemon
   私有协议（`internal/server/procscan.go`）：
   - 桌面会话：命令行 `--session-id <uuid> --agent` 直接给身份，transcript 补目录/标题；
   - TUI：有控制终端的裸 `claude` 进程，按 cwd + 进程启动时间（−90s 容差）配对
     未被占用的最新 transcript；配不上（开了终端还没说过话）落为**匿名行**
     `proc-<pid>`，列表虚线不可点、船台只显空态；它一开口，下一轮 30s 轮询自动
     配成真 sid，前端 8s 静默轮询检测到匿名行消失即跳转转正。
   - 两个环境坑：`ps lstart` 随 `LC_TIME` 变语言/段数（中文环境输出
     「日 10/25 17:46:48 2026」），调 ps 必须强制 `LC_TIME=C LANG=C`；
     目录视图合并 extras 时以 transcript 自证 cwd 为准，防止串目录。
2. **任务可以直接新开一支船队**：`POST /tasks/{id}/launch` 在指定可信目录跑
   `claude --bg --output-format json <开场白>`。2.1.310 实测回执是真人横幅
   「backgrounded · `<短id>`」而非 JSON（即便要求 json），短 id 再轮询
   `claude agents --json` 反查完整 sessionId（UUID），随后等 transcript 落盘、
   绑定任务、跳新船台。目录未被信任等 CLI 拒绝时，错误原文带回落回任务页。
3. **锚地任务可删**：删除放行条件从「仅草稿」放宽到「草稿 + anchored」；
   已派船出港（ready 之后）仍然不可直接删。
