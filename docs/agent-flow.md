# Agent 工作流：Claude Code / Codex 如何与思港协作

思港采用 **pull（拉取）模型**：agent 不是 7×24 守护进程，而是在会话启动时
自己来港口领活。人类负责把念头澄清成任务、标注**重要程度与目标**；agent 负责
领取、叙事式同步进展、对照验收标准归港。

## 一次完整协作

```bash
# 0.（可选）声明身份，一次会话设置一次
export PORT_ACTOR=claude@studio      # codex@studio / 你的名字@studio

# 1. 拉：现在最重要的活是哪个？
port next                 # 按重要程度（关键 > 重要 > 普通 > 也许）输出
port next --json          # 给程序用

# 2. 读：领取前先看完整上下文
port show T0002           # 描述 + 验收标准 + 历史日志
port claim T0002          # 原子领取（flock），两个 agent 不可能撞单

# 3. 干：进度只有两种「有意义的单位」——叙事日志、验收勾选
port log T0002 "int8 基准跑完，单帧 27.4ms，满足预算"
port check T0002 1        # 勾选第 1 条验收标准
port block T0002 "等 Vs3 真机实测数据"   # 卡住立刻上浮，必须给原因
port unblock T0002

# 4. 归港：全部验收项完成后
port done T0002
```

## 设计原则

1. **Pull，不是 push**：人建任务并标重要程度，agent 用 `next` 自取方向，
   不做消息队列、不做派单推送。
2. **目标导向，不是进度导向**：没有截止日期、没有紧急度、没有完成百分比。
   任务只有重要程度（`critical/high/normal/someday`）与所服务的目标（`goal`）。
   agent 选活只问「什么对目标最重要」，不问「什么快逾期了」。
3. **进度 = 验收清单 + 叙事日志**，不是心跳、不是百分比：
   - `port check` 勾选「完成定义」中的具体条目；
   - `port log` 记录过程，人和其他 agent 能实时（SSE）看到发生了什么。
4. **撞单不可能**：claim 是锁内状态机推进，重复领取直接报错；
   agent 崩溃留下的 in_progress 由人工 `reclaim`（规划中）处理，不用租约心跳。
5. **JSON 是给 agent 的一等输出**：`list/show/next` 全部支持 `--json`，
   正文与元数据分离——agent 同步状态只碰 frontmatter，永不解析正文。

## HTTP API（daemon 已在跑时）

同一套数据的另一个入口，适合非 shell 环境或远程 agent：

| 方法与路径 | 作用 |
|---|---|
| `GET /api/next` | 最重要的 ready 任务 |
| `GET /api/tasks?status=ready` | 任务列表（按重要程度排序） |
| `POST /tasks/{id}/claim` | 领取（身份走 cookie/表单，v0.2 从简） |
| `POST /tasks/{id}/done` `/block` `/unblock` `/log` `/check` | 状态推进 |

CLI 与网页走的是同一个 store：**无论哪边写入，另一边 SSE 实时刷新**。

## 给 agent 的提示词模板（可贴进 CLAUDE.md / 系统提示）

```
本项目任务通过思港 MindPort 同步（CLI: port，vault: ~/MindPort）。
开工前先 `port next` 选最重要的 ready 任务，`port show` 读上下文，
`port claim` 后再动手。每完成一个有意义的步骤 `port log` 记录；
满足一条验收标准就 `port check T### n`；全部完成才 `port done`。
遇到阻塞 `port block T### "原因"`，不要静默卡住。
```
