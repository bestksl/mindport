# ADR-0001：本地优先 · 单机拓扑与连接方式

- 状态：**Accepted**
- 日期：2026-10-25
- 关键词：local-first, single-machine, daemon, Tailscale, SSH, git

## 背景

思港（MindPort）是人与 AI agent 共用的任务调度系统。立项时确认的部署事实：

1. **所有 agent（Claude Code、Codex）都运行在用户本人的 Mac Studio 上**，即与任务库同一台机器；Mac Studio 长期开机、固定电源。
2. 任务库需要能被人类用 Obsidian / 任意编辑器直接阅读，因此任务以 Markdown 文件存放。
3. 用户不在 Studio 前时的远程需求有两类：
   - **B. 手机快速录念头**（外出时把想法丢进 inbox，以捕获为主）；
   - **C. 笔记本通过 SSH 登录 Studio 干活**（完整命令行能力）。
4. 不存在「远程服务器上的 agent」这一场景；多 agent 实时抢单的分布式冲突不存在。

## 决策

采用**本地优先、单机为中心**的架构，分四层演进，每一层都独立可用，且不改变下层的数据格式：

```
┌──────────────────── Mac Studio（常开）────────────────────┐
│                                                           │
│   Vault：~/MindPort/  Markdown 任务文件（唯一主存储）       │
│     ▲        ▲        ▲                                   │
│     │ 本机文件 I/O（加锁）                                 │
│  port CLI   Claude Code   Codex        ← 都是本机进程      │
│     ▲                                                    │
│     │ localhost                                          │
│  port serve（daemon：HTTP API + Web/PWA）                 │
│     ▲                                       ↑ 系统 sshd  │
│  Tailscale 虚拟内网                          │ 端口 22     │
│     ├─ 手机浏览器：录入 / 看板（需求 B）      │            │
│     └─ 笔记本浏览器（可选）                   └─ 笔记本 SSH │
│                                            （需求 C）      │
│                                                           │
│   git remote（GitHub 私有仓）：备份 / 历史 / 未来加机器     │
└───────────────────────────────────────────────────────────┘
```

### 第 1 层：文件 + CLI（地基）

- Vault 是一个普通目录（默认 `~/MindPort/`，可用环境变量/配置覆盖），任务为带 YAML frontmatter 的 `.md` 文件；
- `port` CLI 直接读写文件，所有变更通过**本地文件锁**（`flock` 或原子 rename 锁文件）串行化；
- agent 与人类使用完全相同的 CLI，不享受特权通道；
- 同时提供 `--json` 输出，方便 agent 稳定解析。

### 第 2 层：daemon + Web（满足需求 B）

- `port serve` 在本机启动常驻服务：HTTP API（localhost）+ 极简 Web/PWA；
- 手机通过 **Tailscale**（虚拟内网，端到端加密，免费档够用）访问 Studio 的 Tailscale IP，浏览器即开即用，**不做原生 app**；
- Web 的 v1 只做两件事：**快速录入 inbox** 和 **只读看板/任务详情**；编辑类操作仍以 CLI 为主；
- 关键原则：**daemon 只是文件的一扇门，文件永远是主存储**。daemon 挂掉、卸载，数据与 git 历史完好。

### 第 3 层：SSH（满足需求 C）

- 直接启用 macOS 自带「远程登录」（系统 sshd），笔记本用密钥登录后执行 `port`，**零自研网络代码**；
- 推荐同时接入 Tailscale，使 SSH 不依赖公网/端口转发：`ssh <studio-tailscale>`；
- 自研 `port serve-ssh`（独立于系统账号的内置 SSH 服务）**推迟**：仅当未来需要给不受信的 agent 发放「只能领任务」的收敛权限密钥时再评估。

### 第 4 层：git 同步（备份与未来）

- Vault 本身是 git 仓库，`port sync` = 拉取→合并→提交→推送；远程为 GitHub 私有仓；
- 现阶段 git 的角色是**备份、历史审计、未来加 MacBook/换机**，不承担实时协作；
- 未来若出现第二台常驻机器，再引入基于锁文件的撞单防护与同步协议。

## 连接矩阵

| 谁 | 从哪里 | 通道 | 能做什么 |
|----|--------|------|----------|
| Claude Code / Codex | Mac Studio 本机 | 文件 I/O / `port` CLI | 全部操作 |
| 用户本人 | Studio 前 | CLI / Obsidian / Web | 全部操作 |
| 用户手机 | 任何网络 | Tailscale → Web/PWA | 录入 inbox、查看看板（v1） |
| 用户笔记本 | 任何网络 | Tailscale → 系统 SSH | 全部 CLI 操作 |
| （未来）第二台机器 | — | git clone + `port sync` | 离线协作（暂不实现） |

## 安全模型

- daemon **只绑定 Tailscale 接口与 loopback**，绝不监听公网；不做公网端口转发、不做裸 ngrok；
- Web/API 首次启动生成访问 token（写在本机配置文件，权限 600），手机端登录一次；
- SSH 走系统账户与密钥鉴权，遵守 macOS 现有安全边界；
- git remote 为私有仓，凭据走系统钥匙串，不落盘；
- 所有写操作（无论来自 CLI、daemon 还是 SSH 会话）最终都汇聚为同一种「加锁 → 改文件 → 记日志」的本地操作，审计口径统一。

## 明确不做（现阶段）

- ❌ 中心服务器 / 云托管：Studio 常开，「关机也要在手机录念头」不是真需求；Tailscale + PWA 已覆盖 B；
- ❌ 原生手机 app：PWA 足够，且免双端维护；
- ❌ 自研 SSH 服务、分布式锁、多活同步：没有第二台常驻机器，均为过度设计；
- ❌ 公网暴露 / 域名 / HTTPS 证书：Tailscale 已加密，不额外开攻击面。

## 后果

- 优点：v1 只需实现「文件格式 + CLI」即可让全部本机 agent 开工，daemon 与 SSH 不阻塞地基；无运维成本；数据零锁定。
- 代价：手机能力依赖 Studio 在线（已确认常开）；离开 Tailscale 无法录入（v1 接受，未来可做 PWA 离线缓存 + 上线补传）。

## 待定

1. **实现语言**：拟采用 **Go**——单静态二进制分发（`brew install` 即可）、CLI / HTTP daemon / 未来内置 SSH 都是一等公民、交叉编译简单；待确认。
2. Vault 目录结构与任务文件格式（frontmatter schema、ID 规则、状态机、锁文件格式）→ ADR-0002。
3. daemon API 形态（REST/JSON 为主，是否需要 WebSocket 做看板实时刷新）→ ADR-0003。
