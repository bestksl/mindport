# ADR-0003：Vault 结构与文件格式规范（v0 草案）

- 状态：**Proposed**（未经代码验证，实现首个 CLI 时允许修订并升版）
- 日期：2026-10-25
- 相关：[ADR-0001](0001-local-first-single-machine.md)、[ADR-0002](0002-language-go.md)

## 目标与原则

1. **文件即数据库**：每条念头/任务/笔记是一个带 YAML frontmatter 的 Markdown 文件，Obsidian 与任意编辑器可直接读写，不依赖 `port` 也能看懂全部数据。
2. **一个信封，三种体型**：`.md` 正文承载自由文字（思绪、笔记、描述、日志），文件头部的 YAML frontmatter 承载机器字段（状态、归属、时间戳）。agent 同步状态**只读写 frontmatter，永不解析正文**，因此正文怎么写都不会污染任务状态。
3. **人类手写与程序写入等价**：frontmatter 缺失/多余字段时解析从宽；`port doctor` 负责体检与修复建议。
4. **一切写操作串行化**：单机构架下，进程间用一把文件锁保证原子，不引入数据库服务。
5. **正文自由，元数据受控**：frontmatter 是 `port` 的契约区；`port log` 只在约定锚点追加，不改正文其他内容。
6. **持久真相与运行时状态分离**：思绪/任务/笔记是持久对象，入 git；高频易变的运行时状态（心跳、在线状态、pid）放 `.port/state/` 且 gitignore，永远不是真相来源（详见后文）。
7. **一对象一文件**：不做「一个 queue.yaml 装全队任务」——多 agent 并行写同一文件必然冲突；单文件隔离冲突面，双链以 T 编号为锚、不绑文件名。
8. 时间一律 RFC 3339 带时区（如 `2026-10-25T18:30:00+08:00`）。

## Vault 目录结构

```
~/MindPort/                     # 路径可由 PORT_VAULT 环境变量或 --vault 覆盖
├── inbox/                      # 闪念：未澄清的想法，只捕获、零负担
│   └── 20261025-1830-量化模型耗时.md
├── tasks/                      # 正式任务，扁平存放（状态在 frontmatter，不靠目录分流）
│   └── T0001-评估量化模型在Vs3上的耗时.md
├── notes/                      # 自由笔记，同样信封；可用 [[T0001]] 与任务双链
│   └── 20261025-量化背景知识.md
├── archive/                    # done/cancelled 经 `port archive` 移入（纯整理，不删数据）
├── attachments/                # 图片等二进制（手机拍照录入在后期使用）
└── .port/
    ├── config.yml              # vault 级配置（assignee 别名、默认时区等）
    ├── counter                 # 纯文本，下一个任务序号：当前值 "17"
    ├── state/                  # 运行时易变状态，.gitignore：心跳/在线/pid
    └── locks/
        └── vault.lock          # 写事务全局 flock 文件（不入库或入库均可）
```

- inbox 文件名：`YYYYMMDD-HHMM-<标题截断>.md`，同分钟冲突时追加 `-2`；它没有任务 ID。
- tasks 文件名：`T<四位序号>-<slug>.md`，slug 取标题、可空；**标题修改时允许重命名文件，T 编号永不变**，跨文件引用一律以 T 编号为准（Obsidian 链接以 `[[T0001]]` 形式解析，不绑文件名）。
- notes 文件名：`YYYYMMDD-<slug>.md`，无 T 编号；笔记之间、笔记与任务之间用 `[[…]]` 互链。

## 为什么是 MD + YAML frontmatter

| 候选 | 结论 | 原因 |
|------|------|------|
| MD + YAML frontmatter | ✅ 采用 | 机器字段结构化直读；正文自由；Obsidian/git 原生 |
| 纯 YAML | ❌ | 思绪与笔记的长文退化为转义字符串，不可手写、Obsidian 中不可读 |
| 纯 MD（无 frontmatter） | ❌ | 状态要从正文正则解析，agent 同步极脆弱 |
| md + meta.yaml 副车 | ❌ | 双文件必然出现不一致，且 Obsidian 双链断裂 |
| SQLite / 内嵌数据库 | ❌ | 二进制不可 diff、不可手改、数据被程序绑死，违背原则 1 |
| 单个 queue.yaml 聚合全队任务 | ❌ | 多 agent 并行写 = 冲突热点；单文件无并行、无独立链接 |

frontmatter 本身就是 YAML——任务队列所需的结构化能力（status / assignee / 时间戳 / 依赖 / refs）一项不少，只是把「人读的部分」还给了 Markdown。

## 对象一：闪念（thought）

inbox 条目字段从简，捕获动作三秒内完成：

```markdown
---
type: thought
created: 2026-10-25T18:30:00+08:00
source: mobile            # mobile | cli | web | editor，可省略
tags: []
---

洗澡时想到：量化模型是不是可以只在 Vs3 上测 int8 一档？
```

`port promote <file>` 将闪念转为正式任务：分配 T 编号、移入 `tasks/`、进入 `ready`，正文原样保留。

## 对象二：任务（task）

```markdown
---
id: T0001
type: task
title: 评估量化模型在 Vs3 上的耗时
status: in_progress
priority: normal          # low | normal | high | urgent
assignee: claude@studio   # 未领取为 null
tags: [model, benchmark]
created: 2026-10-25T18:30:00+08:00
updated: 2026-10-25T19:02:00+08:00
due: null
claimed_at: 2026-10-25T19:00:00+08:00
completed_at: null
depends_on: []            # [T0000, ...]
refs: []                  # 外部关联：repo/branch/pr/url，自由键值
---

## 描述
对比 fp32 / int8 / int4 三档量化在 Honor Vs3 上的单帧耗时。

## 验收标准
- [ ] int8 单帧 ≤ 30ms
- [ ] 三档结果记入本任务日志

## 日志
- 2026-10-25 18:30 +0800 — created by port
- 2026-10-25 19:00 +0800 — claimed by claude@studio
- 2026-10-25 19:02 +0800 — int8 基准完成，27.4ms；结果见 attachments/int8.csv
```

字段约定：

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | ✅ | `T%04d`，全 vault 唯一，由 `.port/counter` 在锁内分配 |
| `type` | ✅ | `task` / `thought` |
| `title` | ✅ | 单行标题；同步进正文 H1 可选，以 frontmatter 为准 |
| `status` | ✅ | 见状态机 |
| `assignee` | ✅（可 null） | 执行者标识：`human`、`claude@studio`、`codex@studio` 等，别名表在 config |
| `priority` | ✅ | 默认 `normal` |
| `created` / `updated` | ✅ | 每次写事务刷新 `updated` |
| `claimed_at` / `due` / `completed_at` | – | 时间戳或 null |
| `tags` / `depends_on` / `refs` | – | 数组；`refs` 元素为自由键值对象（`repo`, `branch`, `pr`, `url`） |

## 对象三：笔记（note）

与思绪同属「薄元数据 + 自由正文」，但它是**长期沉淀**，不进 inbox、不参与状态机：

```markdown
---
type: note
title: 量化方案的背景知识
tags: [model, research]
created: 2026-10-25T20:00:00+08:00
updated: 2026-10-25T20:00:00+08:00
---

int8 为什么在 NPU 上反而可能更快……（任意长文、图片、双链）

关联任务：[[T0001]]
```

- 笔记没有编号压力，`port note "<标题>"` 以日期文件名落盘即可；未来若需要稳定引用，可再引入 `N%04d`（不影响任务编号空间）。
- 任务也可反向链接笔记；解析双链时 T 编号优先匹配 tasks/，其余按文件名匹配 notes/ 与 inbox/。

## 状态机

```
            promote                 claim                done
 thought ───────────▶ ready ──────────────▶ in_progress ───────▶ done
                        │                       │  ▲             
                        │                       │  │ unblock     
                        │      block            ▼  │             
                        ├──────────────────▶ blocked ┘
                        │
                        └──── cancel ────▶ cancelled（终态）
```

- v1 状态集合：`ready` / `in_progress` / `blocked` / `done` / `cancelled`。`claim` 一步到位置 `in_progress` 并写 `assignee`、`claimed_at`（不单设 claimed 态，减少空转）。
- `done` / `cancelled` 为终态；`port reopen T0001` 可退回 `ready` 并清空完成字段（留日志）。
- `blocked` 必须带一条说明原因的日志；解除阻塞回到 `in_progress`。
- 非法跳转由 CLI 拒绝（如 `done` 一个 `ready` 任务需先 claim）。
- `depends_on` 中存在未完成任务时，`claim` 被拒绝（`--force` 可覆盖并留痕）。

## 并发与锁协议

1. **事务锁**：所有写命令先 `flock(LOCK_EX)` `.port/locks/vault.lock`，在锁内完成「读文件 → 改 → 原子写回（写临时文件 + `rename`）→ 追加日志 → 刷 counter」。CLI 与 daemon 的 HTTP handler 走同一把锁、同一套函数。
2. **领取即归属**：`claim` 的持久语义就是 frontmatter 里的 `assignee + status`，不需要额外锁表；因为有全局 flock，两个 agent 同时 claim 只有一个成功，失败者收到「已被 X 领走」。
3. **僵尸任务**：agent 崩溃留下 `in_progress` 残留。v1 用人工通道：`port unclaim`（仅本人或 `--force`）与 `port reclaim T0001 --force`（人类使用，留日志）。心跳/租约自动过期推迟到 daemon 阶段（ADR-0004 之后再议）。
4. **手改文件**：允许。下次任一命令在锁内重新解析即可；字段损坏时命令报错并指向 `port doctor`，绝不静默覆盖正文。

## 持久数据 vs 运行时状态

| 数据 | 例子 | 存放 | 入 git | 频率 |
|------|------|------|--------|------|
| 持久对象 | thought / task / note | 各自目录的 `.md` | ✅ | 低，人为或状态变化驱动 |
| 运行时状态 | agent 心跳、在线 presence、daemon pid、锁的 stale 标记 | `.port/state/*.yaml` | ❌ gitignore | 高，秒级 |

- 任务归属的**唯一真相是任务文件 frontmatter 里的 `assignee/status/claimed_at`**；心跳只是「这人还活着」的提示，崩溃后用前文的 unclaim/reclaim 人工通道处理，心跳永不会自动改写归属。
- 这条边界保证：git 历史只记录有意义的变化，不会被秒级心跳刷爆；全局锁也不会被高频心跳长期占用。

## CLI 与文件操作映射（v1 命令面）

| 命令 | 文件效果 |
|------|----------|
| `port add "<标题>"` | 写 `inbox/YYYYMMDD-HHMM-*.md` |
| `port inbox` | 列 inbox/ |
| `port promote <file>` | 分配 T 编号、转写为 task、移入 tasks/ |
| `port new "<标题>"` | add + promote 一步到位（agent 常用） |
| `port list [--status …] [--assignee …] [--json]` | 扫描 tasks/，解析 frontmatter 输出表格/JSON |
| `port claim T0001 [--by codex@studio]` | 锁内状态机推进 + 归属字段 + 日志 |
| `port block / unblock / done / cancel / reopen` | 对应状态跳转 |
| `port log T0001 "<内容>"` | 在 `## 日志` 锚点下追加一行 |
| `port note "<标题>"` | 写 `notes/YYYYMMDD-*.md`（薄 frontmatter + 空正文） |
| `port doctor` | 校验编号连续/状态合法/链接存在/frontmatter 完整 |
| `port sync` | git pull --rebase → 自动提交变更 → push（git 层冲突策略另文） |

## 待定（实现时决策）

- frontmatter 解析库的选择与「保留键顺序/注释」策略（影响手改文件后的 diff 干净程度，是体验关键）；
- 大附件是否走 git LFS；
- 正文模板的本地化程度（日志行格式固定为 `- 时间 — 内容`，其余小节允许自由增减）。
