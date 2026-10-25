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

## 设计文档

架构决策记录见 [`docs/adr/`](docs/adr/README.md)：

- **ADR-0001** 本地优先 · 单机拓扑：CLI 直接读写文件，Android 手机走 Tailscale + Chrome PWA，笔记本走系统 SSH，git 负责备份；
- **ADR-0002** 实现语言：Go（单二进制，CLI / daemon / 内嵌 Web 一体）；
- **ADR-0003** Vault 结构与文件格式 v0：Markdown + YAML frontmatter 统一信封，thought / task / note 三类型，T 编号、状态机、flock 锁协议，持久数据与运行时状态分离。

## 状态

🚧 **Pre-alpha · 孵化阶段**。仓库目前只有这份宣言与架构决策，设计讨论与任务拆解将在 Issues 中进行——欢迎把你闪过的念头也丢进来。

---

<p align="center">
念头会消失，但港口一直在。
</p>
