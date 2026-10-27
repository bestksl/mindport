# ADR-0008 会话船台：transcript 全量桥接与双向回话

- 状态：Accepted（2026-10-26）
- 接续：[ADR-0007](0007-agent-integration.md)（官方发现口定会话生死）、[ADR-0005](0005-harbor-dispatch.md)（调度港模型）

## 背景

v0.4 的调度闭环在「派船之后」断了：网页只能看到心跳状态行（在线/工作中/在忙 T 编号），
看不到船**正在做什么**，也没有给船**回话**的通道。船不主动 `port log` 时，派船后是黑盒；
人想中途补一句指令，没有投递口。

诉求明确为：点开船队里任意一艘船（会话），看到**幂等、可滚动全量**的实时会话流，
并能继续输入（人 → 船）。

## 结论

Claude Code 没有「attach 旁观运行中会话」的官方 API，但给了两个受支持的接触面，
组合起来恰好够用：

1. **transcript jsonl 是官方逐行追加的全量会话流**：
   `~/.claude/projects/<编码后的 cwd>/<sessionId>.jsonl`。
   `claude agents --json`（ADR-0007）已提供 `sessionId + cwd`，定位文件是直球。
   assistant 的 tool_use 在工具开跑前落盘、tool_result 完成后落盘，
   tail 文件即可得到秒级实时时间线（非逐字 token 流——那只属于自己拉起的
   `claude -p --output-format stream-json` 进程；动作级粒度对调度跟踪足够）。
2. **`claude -p --resume <sessionId>` 是官方续话口**：在同一会话上追加一轮，
   历史上下文全在，新内容继续写同一个 transcript 文件 → 时间线自动长出回复。
   busy 中的会话不能并发 resume（官方拒绝双写），此时留言进船位信箱，
   经 UserPromptSubmit 钩子以 `additionalContext` 在船下一轮自动注入。

不做的事：不 attach/注入运行中进程、不模拟键盘、不读 API key、不改写 transcript。

## 设计

### 数据源与编码

- 目录编码复刻 Claude Code 自身规则：cwd 绝对路径中每个非 `[A-Za-z0-9]`
  字符替换为 `-`（如 `/Users/jmzx/x/.claude` → `-Users-jmzx-x--claude`）。
- 只解析 `type ∈ {user, assistant}` 且带 `message.content` 的记录；
  忽略 api-request/blob、mode、snapshot 等内部噪音（实测一份长会话 1 万余行记录里
  这两类只占约四成，且字段不稳定，不依赖）。
- content block 映射：`text` → 文本气泡；`thinking` → 折叠思考块；
  `tool_use` → 动作卡（工具名 + 白名单摘要：Bash.command / Edit,Write.file_path /
  TodoWrite.todos 等）；user 侧 `tool_result` 按 `tool_use_id` 挂到对应动作卡
  （退出码、耗时、折叠原文）。

### 幂等全量与滚动

- 每条消息带 `uuid`，前端按 uuid 去重；增量按**字节偏移**拉取
  （`?from=<offset>` 读到 EOF，返回新 offset），重复请求结果一致，天然幂等。
- 大文件（实测长会话 60MB+）**不整读**：首屏只解析最后 128KB，
  向上滚动触发 `?before=<offset>` 按 128KB 块前翻，行边界对齐；
  可一直滚到文件头，即「全量」。
- 轮询而非长连：会话页打开时每 1.5s GET 增量（页面隐藏即暂停），
  与既有「小数据高频轮询、任务变更走 SSE」的分工一致；不新建每会话 watcher。

### 双向输入（`POST /sessions/{sid}/say`）

- 会话 idle（发现口快照 status≠busy）：serve 以会话 cwd 为工作目录执行
  `claude -p --resume <sid> --bg "<留言>"`，后台续跑，立刻返回，回复经时间线可见。
- 会话 busy：留言落 `.port/state/mail/<sid>.jsonl`（未投递），
  前端显示「已放入船位信箱，船下一轮自动收到」。
- 新增隐藏命令 `port mail-pull`（UserPromptSubmit 钩子）：读该会话未投递留言，
  以官方 hook JSON 的 `hookSpecificOutput.additionalContext` 注入下一轮上下文，
  随后标记 delivered（幂等：重复触发不重复注入）。
- 安全边界：resume 只能「追加一轮」，不能强杀/打断船正在跑的动作
  （那需要终端 Ctrl-C）；信箱保证的是「下一个动作之前必看到」。

### 鉴权（可选入港证；默认关闭）

> **修订（2026-10-27，拍板当天）**：初版设计为「serve 一律强制鉴权」，
> 实际使用中手机每次被登录页打断，用户拍板改回**默认内网免证敞开**：
> Tailscale（100.64/10，未开 Funnel）与家庭 WiFi 都是单用户网络边界，
> 鉴权降为 `serve --auth` / `PORT_AUTH=1` 的可选开关。代码与全部测试保留。

transcript 含命令、代码片段、思考与可能的密钥，而 serve 常绑 `0.0.0.0`：

- 仅 `--auth` 启用时，serve 在 `.port/state/web.token` 无则生成 32B 随机
  hex（0600），启动横幅提示用 `port web-token` 查看；`port web-token`
  命令本身始终可用（提前取钥）。
- 启用后除静态资产与 `/login`、`/logout` 外全部路由需要 cookie
  `mp_key=<token>`；未登录访问一律 302 到 `/login`（HTML）或 401（/api/）。
  未启用时中间件整体放行，`/login` 自身 303 回首屏，顶栏不显示登出口。
- 无鉴权 + `0.0.0.0` 的风险（同网段可读全部内容）由启动横幅明示。
- 单用户内网模型，token 即密钥（不做用户体系）；cookie 30 天、HttpOnly、
  SameSite=Lax、HTTP 本机不加 Secure（Tailscale/局域网场景）。
- 程序化构造的 Server（测试）不启用鉴权。

### 隐私默认折叠

时间线默认只给「调度视角」：thinking 折叠为「💭 思考块」、
工具输入/输出折叠为动作行（退出码与耗时外露），点开才看原文；
服务端对单块原文截断 4KB。不做内容级密钥脱敏（不可靠），靠折叠兜底，
需要时再以 `--auth` 加网络内的第二道防线。

### 组件

- `internal/transcript`：路径编码、jsonl 解析、按字节范围读取、块白名单摘要。
- `internal/server`：auth 中间件/登录页；`/sessions/{sid}` 页面；
  `/api/sessions/{sid}/stream|history`；`/sessions/{sid}/say`。
- `cmd/port`：`port web-token`、`port mail-pull`；`serve --auth` 时开鉴权。
- `.port/state/mail/<sid>.jsonl`：船位信箱（运行时态，不进任务文件、不进 git）。

## 后果

- 派船后可全程旁观：动作、文件、命令成败、思考节奏；能中途插话/叫停（下一轮生效）。
- 覆盖面 = 本机全部 Claude Code 会话（无需逐项目装钩子即可**看**；
  信箱注入需要 UserPromptSubmit 钩子，沿用 ADR-0007 的心跳钩子同款安装方式）。
- 依赖 transcript 文件格式这一未成文接口：解析严格容错（坏行跳过、字段缺席降级），
  格式漂移时最坏是某些块不显示，不影响任务主链路。
- Codex 等其他 agent 无此 transcript，时间线只对 Claude 会话成立。

## 修订（2026-10-27）：扫盘模式与零输入聚合

初始版只能从船队/心跳进入会话，历史船「看不见也想不起」。追加两层：

- `GET /sessions?dir=<项目绝对路径>`：不依赖心跳，直接扫
  `~/.claude/projects/<编码目录>/`（300 文件上限），列出该项目全部 transcript，
  `resolveSession(sid, dirHint)` 以「绝对路径 + transcript.Exists」自证兜底，
  船台/stream/history/say 全认，离港历史照样实时流与 `--resume`。
- 空态聚合：不给路径时扫全部编码目录，列最近 40 个会话。真实项目路径不靠
  可逆编码，而由 transcript 每条消息顶层 `cwd` 自证恢复——只取文件头（真人首句
  当标题）与文件尾（最后 cwd＝当前目录、末事件时间）各 64KB，规避跨目录
  `--resume` 时头部 cwd 过期，也不全文扫描大文件（实测 500ms→6ms）。
  无 cwd 的老文件不展示；聚合行不给消息计数，单目录视图才全文计数。
- 标题过滤 compaction 续接句、图片占位、系统提醒等非真人「伪首句」。
- `GET /api/sessions` 轻量 JSON，列表 8s 静默轮询，只切行状态 class，
  页签隐藏自动停；状态徽标由 CSS 按行 class 显隐，轮询不触碰滚动位置。
- 输入仍可手贴（支持 `~/` 展开、datalist 补全、localStorage 最近目录），
  但已非必经路径。
