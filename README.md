# 思港 · MindPort

> 给每一个闪过的念头，一个可以停靠的港口。

思港（MindPort）是一个**本地优先的任务管理软件**，也是一个**人与 AI 共用的任务台**。命令行名为 `port`。

---

## 这是什么

想法大多诞生在某个瞬间——洗澡时、路上、临睡前。那个瞬间没有接住它，往往要很久以后才重新想起：「那件事，我本来要做的。」

思港为这些瞬间准备了一个低摩擦入口：

- 念头随时**记下来**，零摩擦地先存成草稿，不丢、可搜、可链接；
- 草稿完善好验收标准后进入**待办区**，等待指派；
- **你来指派**：指定哪个助手干，它**领取**后开始，干完必须**交付结果**，你在任务台验收，全程留痕。

执行者是 AI 助手。**Claude Code、Codex 这样的 AI agent 就是一位位助手**：只领取派给自己的任务，干活、回报结果。指派是定向命令——没有公共任务池，别人看不到也领不走，也不能代操：领取和完成只能由助手在 CLI 完成，网页任务台只能指派/改派/验收/批注。

---

## 为什么叫思港

**思港 · MindPort · `port`**

- **思**——每一个转瞬即逝的思维瞬间；
- **港**——念头停靠的港湾。

视觉上保留了一座像素海港：灯塔、货轮、四套港口配色；业务概念用的是人人都懂的任务、助手、工作目录。

---

## 特性

- 📝 **Markdown 即数据库**：任务是带 frontmatter 的纯文本文件，Obsidian 或任意编辑器可直接打开，数据永远属于你；
- 🤖 **为 agent 而设计**：`port next` 只显示派给自己的任务，`port claim` / `port done` / `port log` 对人类和 AI 同样友好，内置 `--json`；
- 🖥️ **CLI 优先，SSH 原生**：一台机器、一个密钥，远程 agent 即可领取任务干活，无需额外部署；
- 🚢 **命令式指派**：指派是定向的，只有被指定的助手能领取；待办区的任务对其他助手不可见，不能代领；还能指定到具体**工作目录**（某个项目目录里的会话），街拍项目的会话领不到闲鱼的任务；
- 🏁 **完成硬门禁**：验收项必须全勾（或附交付结果），特殊情况强制完成必须写理由、永久留痕；
- 🔀 **本地优先，Git 同步**：断网可用，多机、多人、多 agent 靠 git 协作；
- 🧠 **三秒捕获**：念头三秒记下，直接就是一条草稿任务（占 T 编号），随后从容完善验收、指派，没有独立的闪念/inbox；
- 🎨 **复古像素海港**：4 套命名配色（夜港·木舱暖夜 / 日港·羊皮海图 / 深海·墨蓝冷夜 / 青瓷·青花白瓷）顶栏拼色板切换并记住偏好；助手列表收成浮层；顶栏一行内有像素灯塔与微型海景（集装箱货轮在港内往返掉头、吊机轻吊货箱）；
- 🗑️ **草稿可删**：仅草稿（未发布到待办区）能删除，发布后的任务有流转历史不可抹除；
- 🥸 **防老板皮相**：顶栏 `</>` 一键切到黑白等宽「debug 台」外观（招牌/标题/按钮换英文伪装名），状态只存 sessionStorage——刷新不露馅，关掉标签即恢复；
- ⛵ **实时会话页（ADR-0008）**：助手列表里点任意会话即进它的实时会话页——按字节偏移幂等增量读取 `~/.claude/projects` 下的 transcript jsonl，真人/助手/工具/思考分角色成时间线，可一直向上翻到会话开端（1.5s 轮询、页面隐藏暂停、uuid 去重不丢不重）；思考与工具原文默认折叠；底部可直接给助手**继续留言**：空闲会话经 `claude -p --resume <sid> --bg` 后台续跑，助手正忙时留言排队，由 UserPromptSubmit 钩子在下一轮自动注入（幂等不重放）；
- 🔐 **内网免登录，可选配对上锁**：默认 serve 不设登录（组网/家庭内网单用户模型）；不可信网络用 `serve --auth`（或 `PORT_AUTH=1`）上锁后，电脑上运行 `port pair` 生成 **10 分钟一次性配对码**并在终端显示二维码，手机扫码即完成该设备的配对——每台设备持有独立会话凭证，可用 `port devices` 随时单独吊销，不影响其他设备；兜底方式是 `port web-token` 取访问密钥，在登录页粘贴。任何受信网络之外都不要裸奔，因为网页里能看到会话 transcript（含命令/代码/思考）；
- 🔌 **HTTP / SSH 接口**：任务台 Web 看板已可用，更多集成在规划中。

---

## 一分钟示例

```bash
# 念头闪现，三秒记下（别打断当下；直接得到草稿任务 T001）
port add "评估量化模型在 Vs3 上的耗时"

# 看看任务台里有什么
port list                       # 草稿 / 待办 / 待领取 / 进行中 / 已完成

# 发布到待办区，然后指派给某位助手
port publish T001                          # draft → anchored（其他助手看不到）
port dispatch T001 --to claude@studio      # anchored → ready（定向指派）

# 被指派的助手：领取、干活、交结果
port next                       # 只显示派给我的任务（待办区、别人的任务都不出现）
port claim T001                 # 不是派给你？store 直接拒绝
port log T001 "基准测试完成，结果见 results.csv"
port check T001 1               # 勾选验收项
port done T001                  # 验收全勾才能完成；没有验收项要 --result
```

远程服务器上的 agent，一行命令就能领任务干活：

```bash
ssh build-box "PORT_ACTOR=codex@build-box port claim T001 && port run T001 && port done T001 --result '...' "
```

每个任务就是一个可读的 Markdown 文件，人不装软件也能看：

```markdown
---
id: T001
title: 评估量化模型在 Vs3 上的耗时
status: in_progress
assignee: codex@build-box
claimed_at: 2026-10-25T10:00:00+08:00
tags: [model, benchmark]
---

## 描述
对比 fp32 / int8 / int4 三档量化在 Honor Vs3 上的单帧耗时。

## 日志
- 2026-10-25 10:00 由 codex@build-box 领走
- 2026-10-25 11:20 基准完成，int8 满足 30ms 预算
```

---

## 工作流：人 → 思港 → AI

1. **捕获**：终端、编辑器、手机，任何入口把念头记下来，立即成为草稿任务；
2. **编辑**：在任务单里澄清描述、完善验收标准（仍是草稿，助手看不到）；
3. **指派**：发布到待办区，随后指派给 Claude Code / Codex（定向命令，不可任领）；
4. **领取**：只有被指派的助手能 `claim`，状态实时回写，别的助手抢不走；
5. **完成验收**：验收全勾或附交付结果才能完成，你在任务台对照验收、批注回溯。

---

## 路线图

- [x] 任务文件格式规范（frontmatter schema、状态机、flock 锁协议）
- [x] `port` CLI：`add`（记下即草稿） / `list` / `next` / `claim` / `done` / `log` / `dispatch`
- [x] HTTP API（REST / JSON）与移动优先 Web 任务台（SSE 实时刷新、PWA）
- [x] 设备配对与独立会话（`port pair` 扫码配对、`port devices` 单独吊销）
- [x] SSH 远程使用模式（组网访问，见 [远程使用指南](docs/remote.md)）
- [ ] Git 同步：`port sync` 与多执行者撞单防护
- [ ] 任务台 App 化：tab 导航取代长滚动（T0010）
- [ ] Obsidian 插件（可选）

---

## 远程使用

人在外面、Studio 在家：用任意组网方案（蒲公英、WireGuard 等）把设备组进同一网络，
手机继续用 Web/PWA、笔记本 SSH 进 Studio。serve 启动横幅会分别标出**组网地址**和**局域网地址**。
详见 **[远程使用指南](docs/remote.md)**。Agent（Claude Code / Codex）的领取与进度
同步约定见 **[Agent 工作流](docs/agent-flow.md)**。

## 设计文档

架构决策记录见 [`docs/adr/`](docs/adr/README.md)（ADR 写于早期，文中沿用当时的港口业务隐喻；
产品文案现已改用任务/助手等主流说法，视觉皮肤不变）：

- **ADR-0001** 本地优先 · 单机拓扑：CLI 直接读写文件，Android 手机走组网 + Chrome PWA，笔记本走系统 SSH，git 负责备份；
- **ADR-0002** 实现语言：Go（单二进制，CLI / daemon / 内嵌 Web 一体）；
- **ADR-0003** Vault 结构与文件格式 v0：Markdown + YAML frontmatter 统一信封，thought / task / note 三类型，T 编号、状态机、flock 锁协议，持久数据与运行时状态分离；
- **ADR-0004** 草稿/发布模型与执行者在线心跳；
- **ADR-0005** 定向指派模型：待办区、命令式指派、完成硬门禁；
- **ADR-0006** 快速捕获：取消独立闪念/inbox，捕获直接生成草稿任务。

## 当前可用的最小闭环

CLI + 内嵌 Web（零前端构建，资源 `go:embed` 进同一二进制），网页与 CLI 经
**SSE 实时互刷新**（CLI/agent 一写，手机页面立刻变）。网页是纯任务台
（发布/指派/改派/验收/批注），助手只能在 CLI 领取与完成；
状态流为 `草稿 → 待办 → 待领取 → 进行中 →（受阻）→ 已完成`；
完成有硬门禁（验收全勾或附结果），顶栏显示助手**在线状态**，桌面/直板手机双端响应式。

```bash
go build -o bin/port ./cmd/port
./bin/port init                                   # 初始化 ~/MindPort（--vault / $PORT_VAULT）

# 捕获（记下就是草稿任务）
./bin/port add "洗澡时的闪念"                       # → T0001 草稿（可加 --importance high）

# 编辑 → 发布 → 指派（目标导向：不谈紧急，只标重要程度）
./bin/port new "验证 int8 耗时" --draft --importance critical --goal "Vs3 上 NPU 方案定稿"
./bin/port publish T0001                          # 发布到待办区（其他助手看不到）
./bin/port dispatch T0001 --to claude@studio      # 定向指派：只有 claude 能领取
./bin/port dispatch T0001 --to claude@studio --berth ~/studio/jiepai-app
#                 ↑ 指定工作目录：只在该项目目录的会话能领，防止别的项目领错任务
#                 importance: critical(关键) high(重要) normal(普通) someday(也许)

# agent（Claude Code / Codex）的工作流
./bin/port next                    # 只看派给我的任务（待办区、别人的任务都不出现）
./bin/port show T0001 [--json]     # 读全文：描述/验收标准/历史日志
./bin/port claim T0001             # 领取（PORT_ACTOR 或 --by 指定身份），自动报在线
./bin/port log T0001 "基准 27ms"   # 叙事式进展同步
./bin/port check T0001 1           # 勾选第 1 条验收标准（进度=验收清单）
./bin/port block T0001 "等真机"    # 受阻上浮（必须给原因）/ unblock
./bin/port done T0001              # 完成硬门禁：验收全勾；否则 --result；死锁 --force --reason
./bin/port present [T0001] --note "正在跑测试"  # 手动报在线（通常由会话钩子自动完成）
./bin/port away                    # 在线灯灭
./bin/port list [--status anchored] [--json]

./bin/port serve --host 0.0.0.0 --port 8787
# 默认内网免登录，手机打开即用；不可信网络加 --auth 上锁（PORT_AUTH=1 同义）。
# 上锁后推荐扫码配对，每台设备独立授权、可单独吊销：
./bin/port serve --host 0.0.0.0 --port 8787 --auth
./bin/port pair                   # 另开终端运行：终端显示二维码 + 配对链接（10 分钟一次性）
./bin/port devices                # 查看已配对设备；port devices revoke <id前缀> 吊销
./bin/port web-token              # 兜底：取访问密钥，在手机 /login 页粘贴
# 手机浏览器（同一局域网，或组网地址）：类 app 底部 tab（任务台 / 助手，助手带在线角标），
# 任务台手机端用顶部分段切换 草稿/待办/待领/进行中/完成（桌面端仍是四列看板）；
# 指派/改派下拉、助手在线灯与「正在做」状态行、验收勾选、批注；
# 点助手里任一会话进「会话页」：实时 transcript 时间线 + 上翻全量 + 直接给助手留言；
# 没有任何代助手操作按钮；SSE 实时更新，在线状态 20s 轮询；手机单列大触控区，
# 分段选择记忆在本机，指派/改派需二次确认
```

Claude Code 在本仓开会话时，`.claude/settings.json` 钩子自动报在线/离开。

详见 [Agent 工作流](docs/agent-flow.md)、[ADR-0005](docs/adr/0005-harbor-dispatch.md)、
[远程使用指南](docs/remote.md)与[运维手册](docs/ops.md)（测试/生产 vault 分离、升级流程）。

代码结构：`internal/store`（vault 文件、frontmatter、flock 锁、状态机、
待办区/指派/完成门禁、在线状态、留言队列，含表驱动测试）、`internal/transcript`
（Claude transcript jsonl 的字节偏移增量/上翻读取与容错解析）、`internal/server`
（net/http + go:embed + fsnotify/SSE + goldmark 渲染 + 配对码/设备会话，纯任务台）、
`cmd/port`（cobra 子命令）。

## 技术架构与技术栈

**一个 Go 二进制 = CLI + 内嵌 Web 服务器 + 前端静态资源**，没有数据库、
没有 Node 构建、没有外部服务，`scp` 一个文件就能部署。

| 层 | 选型 | 为什么 |
|---|---|---|
| 语言/运行时 | Go（单二进制，交叉编译零依赖） | CLI、daemon、Web 同一份代码同一进程 |
| CLI | [cobra](https://github.com/spf13/cobra) | 子命令 + `--json`，人和 agent 共用同一入口 |
| 存储 | Markdown + YAML frontmatter 文件 | 数据可读可 grep，Obsidian/编辑器直开；详见 [ADR-0003](docs/adr/0003-task-file-format.md) |
| YAML | `gopkg.in/yaml.v3` | frontmatter 解析/回写 |
| 并发锁 | `gofrs/flock`（文件锁） | 多个 agent / 网页并发写不会撞单 |
| 持久化 | temp-file + rename 原子写 | 任何时刻文件要么旧的完整、要么新的完整 |
| Web | 标准库 `net/http`（Go 1.22 方法路由 + `PathValue`） | 零 Web 框架 |
| 前端 | 原生 HTML template + 一份手写 CSS，零 JS 框架 | `go:embed` 打进二进制；移动端优先响应式 |
| Markdown 渲染 | goldmark | 任务描述在网页渲染，正文仍以 Markdown 存储 |
| 终端二维码 | skip2/go-qrcode | `port pair` 直接在终端画二维码供手机扫 |
| 实时性 | `fsnotify` 监听文件 + SSE 推送 + 250ms 去抖 | CLI/agent/网页任何一端写入，另一端自动刷新 |
| 心跳传输 | 网页 20s 轮询 `GET /api/agents`（**不走 SSE**） | 在线灯高频小数据，与任务变更通道解耦 |
| PWA | manifest + service worker | 手机可「添加到主屏幕」，像 app 一样打开 |

```
浏览器（手机 PWA / 桌面）  ←─ SSE 任务变更 / 20s 轮询助手在线状态 ─┐
                                                              │
Claude Code / Codex ── port CLI（cobra）──┐                    │
人在终端 ──────────────── port CLI ────────┤                    ▼
                                          ├──► internal/store ──► ~/MindPort/
serve 进程（同一二进制内嵌 net/http）──────┘      (flock + 原子写)    tasks/*.md
   fsnotify 监听 tasks/ → 去抖 250ms → SSE 广播                     .port/state/agents/*.json（在线状态）
   每 30s 跑 claude agents --json 定会话生死/简介 ───────────────►  claude CLI（官方发现口）
   会话页：按字节偏移幂等读 ~/.claude/projects/<cwd>/<sid>.jsonl ┄┄► transcript（只读）
   空闲留言：claude -p --resume <sid> --bg；忙碌留言 → .port/state/mail/ → mail-pull 钩子注入
```

状态机：

```
draft ──publish──▶ anchored ──assign(指定助手)──▶ ready ──claim(仅被指派人)──▶ in_progress
  │                                                                      │
  └────────publish(--to 助手名) 直达 ready                  blocked ◀──block(仅承担者)
                                                                  │  └──unblock/受阻改派
                                                                  ▼
                                                                 done（完成硬门禁）
```

## 在线状态机制（agent presence）

目标：任务台顶栏能看到「Claude / Codex 此刻在不在线、正在干哪个任务」，
但**在线状态是运行时数据，不是任务数据**，崩溃要能自愈，不能有写放大。

- **存储**：`.port/state/agents/<actor>/<session>.json`（actor / session_id / 项目目录 dir /
  name 会话简介 / kind / task / note / seen 时间戳），同一位助手在多个项目里的会话各一条；
  旧版扁平文件 `agents/<actor>.json` 仍兼容读取。与任务文件分离，未来 git 同步时排除该目录；
- **在线判定走官方发现口**（v0.5，ADR-0007 A 档）：serve 每 30 秒跑
  `claude agents --json`——官方为 scheduler 等外部程序提供的活会话全集接口，
  含 `sessionId/cwd/name/kind/state`。在全集里=在线，缺席=离线（文件保留、灰显历史会话）；
  找不到 claude 或调用失败时退回 TTL 模型；
- **TTL 2 分钟（回退路径）**：发现口不可用时，`seen` 在窗口内即在线，超时灰显；
- **20 秒防抖**：内容未变的重复报在线不重写文件，钩子可以高频触发；
- **钩子只报在线**（v0.5 起不再 fork 探活守护、无 pidfile）：SessionStart/UserPromptSubmit
  调 `port heartbeat`，从 stdin JSON 取 session_id/cwd 立即建档；生死交给官方发现口，
  不需要 launchd / cron（用户明确不装系统级托管）；
- **顶栏按会话显示**：同一位 claude 在多个项目各开会话时，每枚芯片带
  📍目录、「会话简介」与运行态（工作中/在线待指派/在忙 T00xx），
  离线灰显「上次在线 x 分钟前」；指派下拉也是会话粒度，凭简介+目录勾选上下文；
- **自动维护**：`claim` 自动带任务号（写到该助手最近活动的会话状态上）、`done` 自动清任务号；
- **Claude Code 钩子**（仓库 `.claude/settings.json`，仅新会话生效；仓库默认只装报在线钩子）：

```json
{
  "hooks": {
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "/Users/jmzx/.local/bin/port heartbeat --by claude@studio" }] }],
    "UserPromptSubmit": [
      { "hooks": [{ "type": "command", "command": "/Users/jmzx/.local/bin/port heartbeat --by claude@studio" }] }
    ]
  }
}
```

  **可选加装 `mail-pull`**（会话页的双向留言才需要）：在 `UserPromptSubmit`
  数组里再加一条——

```json
{ "hooks": [{ "type": "command", "command": "/Users/jmzx/.local/bin/port mail-pull" }] }
```

  助手正忙时你在网页上的留言先进队列（`.port/state/mail/<sid>.jsonl`），
  下一轮提交时该钩子幂等拉走并以 `additionalContext` 注入；重放/并发不会重复
  注入（拉取即回标 delivered）。只看时间线、不装这个钩子也完全可用。

  Codex 或其他 agent 没有钩子时，在项目提示里要求开工 `port present`、
  收工 `port away` 即可（见 [Agent 工作流](docs/agent-flow.md)）。

> 仓库自带的是**项目级**钩子（本仓库开会话才上报）；装进用户级
> `~/.claude/settings.json` 后，任何目录的 Claude Code 会话都会自动报在线，
> 指派下拉即可看到全部会话简介。

## 配对与设备管理

`serve --auth` 上锁后，准入方式有两种：

1. **扫码配对（推荐）**：电脑终端运行 `port pair`，生成 10 分钟内有效、只能使用一次的
   配对码，并直接在终端画出二维码（也会打印组网/局域网多条配对链接，哪条通扫哪条）。
   手机扫码后打开 `/pair#code=...`，页面把一次性码换成该设备专属的会话凭证：
   - 配对码只以 SHA-256 哈希存盘，传输走 URL fragment（不进服务端日志）；
   - 每台设备独立 30 天滑动会话，`port devices` 查看（设备名取自浏览器 UA，
     如 iPhone / Mac / Android 手机）、`port devices revoke <id前缀>` 单独吊销；
   - 输错码不消耗好码，登录/配对有按 IP 的频率限制。
2. **访问密钥（兜底）**：`port web-token` 显示共享密钥，在 `/login` 页粘贴；
   登录成功同样签发设备会话，浏览器不再保存密钥本体。旧版直接把密钥写进
   cookie 的方式仍兼容，会自然淘汰。

## 状态

🚧 **Pre-alpha · 孵化阶段**。核心闭环已通：待办区/命令式指派/完成硬门禁、
agent 在线状态、设备配对、CLI ↔ 文件 ↔ 桌面/移动任务台实时同步；设计讨论在 ADR 中进行。

---

<p align="center">
念头会消失，但港口一直在。
</p>
