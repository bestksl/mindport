# 思港 · MindPort

> 给每一个闪过的念头，一个可以停靠的港口。

思港（MindPort）是一个**本地优先的任务管理软件**，也是一座**人与 AI 共用的任务调度港**。命令行名为 `port`。

---

## 这是什么

想法大多诞生在某个瞬间——洗澡时、路上、临睡前。那个瞬间没有接住它，往往要很久以后才重新想起：「那件事，我本来要做的。」

思港为这些瞬间建了一座港：

- 念头随时**入港停泊**，零摩擦地先存下来，不丢、可搜、可链接；
- 念头被整理成集装箱（任务），打磨好验收标准后在**锚地等待派船**；
- **你是调度长官**：点名叫船，船**接令出港**，干完活**归港必须带结果**，你在调度台验货，全程留痕。

执行者是船。**Claude Code、Codex 这样的 AI agent 是港里的船**：只听派给自己的命令，接令、干活、回报结果。派船是强制命令——没有公共任务池，不能抢单，也不能代操：接令和归港只能船自己在 CLI 完成，网页调度台只能派船/改派/验货/批注。

---

## 为什么叫思港

**思港 · MindPort · `port`**

- **思**——每一个转瞬即逝的思维瞬间；
- **港**——念头停靠的港湾，也是调度任务的港口。

港口有调度长官，也有来船：人下命令、验货；脚本、Claude Code、Codex 接令、干活、交结果。

---

## 特性（规划中）

- 📝 **Markdown 即数据库**：任务是带 frontmatter 的纯文本文件，Obsidian 或任意编辑器可直接打开，数据永远属于你；
- 🤖 **为 agent 而设计**：`port next` 只听派给自己的令，`port claim` / `port done` / `port log` 对人类和 AI 同样友好，内置 `--json`；
- 🖥️ **CLI 优先，SSH 原生**：一台机器、一个密钥，远程 agent 即可接令干活，无需额外部署；
- 🚢 **命令式派船**：指派是强制的，只有被点名的船能接令；锚地货对船不可见，不能抢单；
- 🏁 **归港硬门禁**：验收项必须全勾（或附交付结果），特殊情况强制归港必须写理由、永久留痕；
- 🔀 **本地优先，Git 同步**：断网可用，多机、多人、多 agent 靠 git 协作；
- 🧠 **Inbox → 任务**：先无摩擦捕获，再从容整理、打磨验收、派船；
- 🔌 **HTTP / SSH 接口**：调度台 Web 看板已可用，更多集成在规划中。

---

## 一分钟示例

```bash
# 念头闪现，先扔进港（三秒钟，别打断当下）
port add "评估量化模型在 Vs3 上的耗时"

# 看看港里有什么
port inbox
port list                       # 草稿 / 锚地 / 待接令 / 出港中 / 已归港

# 调度长官：放行到锚地，然后点名叫船
port publish T001                          # draft → anchored（船看不到）
port dispatch T001 --to claude@studio      # anchored → ready（派船命令）

# 被指派的 agent：接令、干活、交结果
port next                       # 只显示派给我的货（锚地货、别人的货都不出现）
port claim T001                 # 不是派给你？store 直接拒绝
port log T001 "基准测试完成，结果见 results.csv"
port check T001 1               # 勾选验收项
port done T001                  # 验收全勾才能归港；没有验收项要 --result
```

远程服务器上的 agent，一行命令就能听令干活：

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

1. **捕获**：终端、编辑器、手机，任何入口把念头丢进 inbox；
2. **备货**：思港把念头存为本地 Markdown，澄清为任务、打磨验收标准（草稿）；
3. **派船**：人作为调度长官放行到锚地，随后点名叫 Claude Code / Codex（命令，不可任领）；
4. **接令**：只有被指派的船能 `claim`，状态实时回写，别的船抢不走；
5. **归港验货**：验收全勾或附交付结果才能归港，人在调度台对照验收、批注回溯。

---

## 路线图

- [x] 任务文件格式规范（frontmatter schema、状态机、flock 锁协议）
- [x] `port` CLI：`add` / `inbox` / `list` / `next` / `claim` / `done` / `log` / `dispatch`
- [x] HTTP API（REST / JSON）与移动优先 Web 调度台（SSE 实时刷新、PWA）
- [x] SSH 远程使用模式（Tailscale 组网，见 [远程使用指南](docs/remote.md)）
- [ ] Git 同步：`port sync` 与多执行者撞单防护
- [ ] 调度台 App 化：tab 导航取代长滚动（T0010）
- [ ] Obsidian 插件（可选）

---

## 远程使用

人在外面、Studio 在家：用 Tailscale 组网，手机继续用 Web/PWA、笔记本 SSH 进 Studio。
详见 **[远程使用指南](docs/remote.md)**。Agent（Claude Code / Codex）的领单与进度
同步约定见 **[Agent 工作流](docs/agent-flow.md)**。

## 设计文档

架构决策记录见 [`docs/adr/`](docs/adr/README.md)：

- **ADR-0001** 本地优先 · 单机拓扑：CLI 直接读写文件，Android 手机走 Tailscale + Chrome PWA，笔记本走系统 SSH，git 负责备份；
- **ADR-0002** 实现语言：Go（单二进制，CLI / daemon / 内嵌 Web 一体）；
- **ADR-0003** Vault 结构与文件格式 v0：Markdown + YAML frontmatter 统一信封，thought / task / note 三类型，T 编号、状态机、flock 锁协议，持久数据与运行时状态分离；
- **ADR-0004** 草稿/发布模型与执行者在线心跳；
- **ADR-0005** 调度港模型：锚地、命令式派船、归港硬门禁。

## 当前可用的最小闭环（v0.4）

CLI + 内嵌 Web（零前端构建，资源 `go:embed` 进同一二进制），网页与 CLI 经
**SSE 实时互刷新**（CLI/agent 一写，手机页面立刻变）。v0.4 起：**人是调度长官、
agent 是船**——网页是纯调度台（放行/派船/改派/验货/批注），船只能在 CLI
接令与归港；状态流为 `草稿 → 锚地 → 待接令 → 出港中 →（受阻）→ 归港`；
归港有硬门禁（验收全勾或附结果），顶栏显示船队**在线心跳**，桌面/直板手机双端响应式。

```bash
go build -o bin/port ./cmd/port
./bin/port init                                   # 初始化 ~/MindPort（--vault / $PORT_VAULT）

# 捕获
./bin/port add "洗澡时的闪念"                       # → inbox
./bin/port promote inbox/20261025-2200-xxx.md      # 闪念 → 草稿任务（--importance high）

# 调度长官：备货 → 放行 → 派船（目标导向：不谈紧急，只标重要程度）
./bin/port new "验证 int8 耗时" --draft --importance critical --goal "Vs3 上 NPU 方案定稿"
./bin/port publish T0001                          # 放行到锚地（船看不到）
./bin/port dispatch T0001 --to claude@studio      # 派船命令：只有 claude 能接令
#                 importance: critical(关键) high(重要) normal(普通) someday(也许)

# agent（Claude Code / Codex）的听令工作流
./bin/port next                    # 只听派给我的令（锚地货、别人的货都不出现）
./bin/port show T0001 [--json]     # 读全文：描述/验收标准/历史日志
./bin/port claim T0001             # 接令（PORT_ACTOR 或 --by 指定身份），自动带心跳
./bin/port log T0001 "基准 27ms"   # 叙事式进展同步
./bin/port check T0001 1           # 勾选第 1 条验收标准（进度=验收清单）
./bin/port block T0001 "等真机"    # 受阻上浮（必须给原因）/ unblock
./bin/port done T0001              # 归港硬门禁：验收全勾；否则 --result；死锁 --force --reason
./bin/port present [T0001] --note "正在跑测试"  # 手动报到（通常由会话钩子自动完成）
./bin/port away                    # 在线灯灭
./bin/port list [--status anchored] [--json]

./bin/port serve --host 0.0.0.0 --port 8787
# 手机浏览器（同 WiFi 或 Tailscale）：草稿区、锚地/待接令/出港中/已归港四列、
# 派船/改派下拉、船队在线灯与「正在做」状态行、验货勾选、批注；
# 没有任何代船操作按钮；SSE 实时更新，在线状态 20s 轮询；手机单列大触控区，
# 派船/改派需二次确认
```

Claude Code 在本仓开会话时，`.claude/settings.json` 钩子自动 present/away。

详见 [Agent 工作流](docs/agent-flow.md)、[ADR-0005](docs/adr/0005-harbor-dispatch.md)、
[远程使用指南](docs/remote.md)与[运维手册](docs/ops.md)（测试/生产 vault 分离、升级流程）。

代码结构：`internal/store`（vault 文件、frontmatter、flock 锁、状态机、
锚地/派船/归港门禁、在线心跳，含表驱动测试）、`internal/server`（net/http + go:embed +
fsnotify/SSE + goldmark 渲染，纯调度台）、`cmd/port`（cobra 子命令）。

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
| 实时性 | `fsnotify` 监听文件 + SSE 推送 + 250ms 去抖 | CLI/agent/网页任何一端写入，另一端自动刷新 |
| 心跳传输 | 网页 20s 轮询 `GET /api/agents`（**不走 SSE**） | 在线灯高频小数据，与任务变更通道解耦 |
| PWA | manifest + service worker | 手机可「添加到主屏幕」，像 app 一样打开 |

```
浏览器（手机 PWA / 桌面）  ←─ SSE 任务变更 / 20s 轮询船队心跳 ─┐
                                                              │
Claude Code / Codex ── port CLI（cobra）──┐                    │
人在终端 ──────────────── port CLI ────────┤                    ▼
                                          ├──► internal/store ──► ~/MindPort/
serve 进程（同一二进制内嵌 net/http）──────┘      (flock + 原子写)    tasks/*.md
   fsnotify 监听 tasks/ inbox/ ◄──────────────────────────────────── inbox/*.md
   变化去抖 250ms → SSE 广播                                          .port/state/agents/*.json（心跳）
```

状态机（v0.4）：

```
draft ──publish──▶ anchored ──assign(点名)──▶ ready ──claim(仅承担船)──▶ in_progress
  │                                                                      │
  └────────publish(--to 船名) 直达 ready                    blocked ◀──block(仅承担船)
                                                                  │  └──unblock/受阻改派
                                                                  ▼
                                                                 done（归港硬门禁）
```

## 在线心跳机制（agent presence）

目标：调度台顶栏能看到「Claude / Codex 此刻在不在线、正在干哪票货」，
但**心跳是运行时状态，不是任务数据**，崩溃要能自愈，不能有写放大。

- **存储**：`.port/state/agents/<actor>.json`（actor / task / note / seen 时间戳），
  与任务文件分离，未来 git 同步时排除该目录；
- **TTL 2 分钟**：`seen` 在窗口内即在线，超时自动灰显「x 分钟前在」，
  不需要显式下线，agent 崩溃也自愈；
- **20 秒防抖**：相同 task/note 的重复心跳不重写文件，钩子可以高频触发；
- **保活守护循环**（`port heartbeat`，v0.4）：会话开始时由 Claude Code 钩子
  拉起一个脱离会话的后台循环，**每 45 秒**探测一次 CLI 主进程（`pgrep -x claude`）：
  在就续期（保留任务号/状态行，绕过 2 分钟 TTL），CLI 退出后 45 秒内自动 `away`
  并清理 pidfile。**不需要 launchd / cron——随会话自生自灭**（用户明确不装系统级托管）；
- **自动维护**：`claim` 自动带任务号、`done` 自动清任务号；
- **Claude Code 钩子**（仓库 `.claude/settings.json`，仅新会话生效）：

```json
{
  "hooks": {
    "SessionStart":     [{ "hooks": [{ "type": "command", "command": "/Users/jmzx/.local/bin/port heartbeat --by claude@studio" }] }],
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "/Users/jmzx/.local/bin/port heartbeat --by claude@studio" }] }]
  }
}
```

  Codex 或其他 agent 没有钩子时，在项目提示里要求开工 `port present`、
  收工 `port away` 即可（见 [Agent 工作流](docs/agent-flow.md)）。

> 注意：钩子是**项目级**的——在本仓库开会话才上报。想让任何目录的 Claude Code
> 都报到，需要把同样的 hooks 放进用户级 `~/.claude/settings.json`（按需手动开启）。

## 状态

🚧 **Pre-alpha · 孵化阶段**。v0.4 闭环已通：锚地/命令式派船/归港硬门禁、
agent 在线心跳、CLI ↔ 文件 ↔ 桌面/移动调度台实时同步；设计讨论在 ADR 中进行。

---

<p align="center">
念头会消失，但港口一直在。
</p>
