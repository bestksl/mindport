# 思港 · MindPort

> 给每一个闪过的念头，一个可以停靠的港口。

思港（MindPort）是一个**本地优先的任务管理软件**，也是一座**人与 AI 共用的任务调度港**。命令行名为 `port`。

---

## 这是什么

想法大多诞生在某个瞬间——洗澡时、路上、临睡前。那个瞬间没有接住它，往往要很久以后才重新想起：「那件事，我本来要做的。」

思港为这些瞬间建了一座港：

- 念头随时**入港停泊**，零摩擦地先存下来，不丢、可搜、可链接；
- 念头被整理成任务，在泊位上**等待调度**；
- 合适的执行者把任务**领走出港**，做完后**归港存档**，全程留痕。

执行者不只有人。**Claude Code、Codex 这样的 AI agent，和你一样是进港的船**：自己查看泊位、领取任务、更新状态、回报结果。你在编辑器里写下的一件事，远程服务器上的 agent 通过 SSH 就能领走、开工、回报。

---

## 为什么叫思港

**思港 · MindPort · `port`**

- **思**——每一个转瞬即逝的思维瞬间；
- **港**——念头停靠的港湾，也是调度任务的港口。

港口从不挑剔来船：人、脚本、Claude Code、Codex，都是平等的执行者。

---

## 特性（规划中）

- 📝 **Markdown 即数据库**：任务是带 frontmatter 的纯文本文件，Obsidian 或任意编辑器可直接打开，数据永远属于你；
- 🤖 **为 agent 而设计**：`port claim` / `port done` / `port log`，输出对人类和 AI 同样友好，内置 `--json`；
- 🖥️ **CLI 优先，SSH 原生**：一台机器、一个密钥，远程 agent 即可领单干活，无需额外部署；
- 🔒 **领单加锁**：一个任务不会被两个执行者同时领走，离线回来也能正确合并；
- 🔀 **本地优先，Git 同步**：断网可用，多机、多人、多 agent 靠 git 协作；
- 🧠 **Inbox → 任务**：先无摩擦捕获，再从容整理、排期、指派；
- 🔌 **HTTP / SSH 接口**：GUI、Web 看板、webhook 通知都在规划中。

---

## 一分钟示例

```bash
# 念头闪现，先扔进港（三秒钟，别打断当下）
port add "评估量化模型在 Vs3 上的耗时"

# 看看港里有什么
port inbox
port list --status ready

# Agent 领走一个任务（自动加锁，防止重复领取）
port claim T001

# 记录过程、推进状态
port log T001 "基准测试完成，结果见 results.csv"
port doing T001

# 做完，归港
port done T001

# 同步给所有人和 agent
port sync
```

远程服务器上的 agent，一行命令就能参与：

```bash
ssh build-box "port claim T001 && port run T001 && port done T001"
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

1. **捕获**：终端、编辑器、以后还有手机，任何入口把念头丢进 inbox；
2. **停泊**：思港把念头存为本地 Markdown，永不丢失、随时可查；
3. **调度**：人把念头澄清为任务、标为 ready，指派给人类、Claude Code 或 Codex；
4. **领单**：agent 通过 CLI / SSH 自行领取，状态实时回写，撞单自动防护；
5. **归港**：结果、日志、关联的提交与 PR 一并存档，事后完整可回溯。

---

## 路线图

- [ ] 任务文件格式规范（frontmatter schema、状态机、锁协议）
- [ ] `port` CLI：`add` / `inbox` / `list` / `claim` / `doing` / `done` / `log`
- [ ] Git 同步：`port sync` 与多执行者撞单防护
- [ ] SSH 远程使用模式（含内置 ssh server 的评估）
- [ ] HTTP API（REST / JSON）
- [ ] GUI 与 Web 看板
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
- **ADR-0003** Vault 结构与文件格式 v0：Markdown + YAML frontmatter 统一信封，thought / task / note 三类型，T 编号、状态机、flock 锁协议，持久数据与运行时状态分离。

## 当前可用的最小闭环（v0.3）

CLI + 内嵌 Web（零前端构建，资源 `go:embed` 进同一二进制），网页与 CLI 经
**SSE 实时互刷新**（CLI/agent 一写，手机页面立刻变）。v0.3 起：**人发布、
agent 领取**（网页默认建草稿，打磨验收后发布/指派，agent 只领已发布任务），
网页顶栏显示执行者**在线心跳**，桌面/直板手机双端响应式。

```bash
go build -o bin/port ./cmd/port
./bin/port init                                   # 初始化 ~/MindPort（--vault / $PORT_VAULT）

# 捕获
./bin/port add "洗澡时的闪念"                       # → inbox
./bin/port promote inbox/20261025-2200-xxx.md      # 闪念 → T 编号任务（--importance high）

# 目标导向：不谈紧急，只标重要程度
./bin/port new "验证 int8 耗时" --importance critical --goal "Vs3 上 NPU 方案定稿"
# 网页建任务默认是草稿；CLI 加 --draft 先进草稿，再发布，可指派
./bin/port publish T0001 --to claude@studio
#                 importance: critical(关键) high(重要) normal(普通) someday(也许)

# agent（Claude Code / Codex）的 pull 工作流
./bin/port next                    # 现在最重要的已发布任务（草稿不进 next）
./bin/port show T0001 [--json]     # 读全文：描述/验收标准/历史日志
./bin/port claim T0001             # 原子领取（PORT_ACTOR 或 --by 指定身份），自动带心跳
./bin/port log T0001 "基准 27ms"   # 叙事式进展同步
./bin/port check T0001 1           # 勾选第 1 条验收标准（进度=验收清单）
./bin/port block T0001 "等真机"    # 受阻上浮 / unblock
./bin/port done T0001              # 归港（心跳自动清任务号）
./bin/port present [T0001] --note "正在跑测试"  # 手动报到（通常由会话钩子自动完成）
./bin/port away                    # 在线灯灭
./bin/port list [--status ready] [--json]

./bin/port serve --host 0.0.0.0 --port 8787
# 手机浏览器（同 WiFi 或 Tailscale）：草稿区、看板三列、发布/指派、
# 执行者在线灯与「正在做」状态行、任务详情、勾选验收、日志、编辑、身份切换；
# SSE 实时更新，在线状态 20s 轮询；手机单列大触控区，归港/发布需二次确认
```

Claude Code 在本仓开会话时，`.claude/settings.json` 钩子自动 present/away。

详见 [Agent 工作流](docs/agent-flow.md)、[ADR-0004](docs/adr/0004-presence-and-publish.md)、
[远程使用指南](docs/remote.md)与[运维手册](docs/ops.md)（测试/生产 vault 分离、升级流程）。

代码结构：`internal/store`（vault 文件、frontmatter、flock 锁、状态机、
草稿/发布、在线心跳，含表驱动测试）、`internal/server`（net/http + go:embed +
fsnotify/SSE + goldmark 渲染）、`cmd/port`（cobra 子命令）。

## 状态

🚧 **Pre-alpha · 孵化阶段**。v0.3 闭环已通：草稿/发布/指派、agent 在线心跳、
CLI ↔ 文件 ↔ 桌面/移动 Web 实时同步；设计讨论在 ADR 中进行。

---

<p align="center">
念头会消失，但港口一直在。
</p>
