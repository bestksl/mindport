# ADR-0007：Claude Code 远程/编程化能力的接入边界（架构评估）

日期：2026-10-26
状态：Proposed（评估结论，分阶段实施需另行拍板）
调研基线：Claude Code v2.1.310（本机实测）+ code.claude.com 官方文档

## 背景

思港 v0.5 的「船位」靠自研机制维持：SessionStart/UserPromptSubmit 钩子调
`port heartbeat` 上报会话（sid/cwd/pid），每会话 fork 一个 45s 探活守护循环。
用户要求评估：要不要、以及如何接入 Claude Code 自身的「远程 API 端口」。

先澄清事实：**Claude Code 没有可接入的远程 API 端口**。

- `claude remote-control`（`/remote-control`、claude.ai/code 与官方手机 App）
  **只发出站 HTTPS，经 Anthropic API 中继，不开任何入站端口**，没有公开协议、
  没有第三方凭据；它是给**人**用的远程界面，不是给程序的 RPC。本机实测
  daemon 进程无任何 TCP LISTEN。
- `/tmp/cc-daemon-501/` 下的 `control.sock` / `*.pty.sock` / `*.claim.sock`
  是后台会话监督进程的**私有 IPC，文档明确不保证稳定**，不能依赖。

但官方另有一组**文档化的编程面**，按稳定性排列：

| 能力 | 接口 | 稳定性 | 能做什么 |
|---|---|---|---|
| 会话发现 | `claude agents --json` | agent view 整体为 research preview，但该命令被文档**明确点名**供 status bar / **scheduler** / 外部监督者读取 | 列出全部交互与后台会话：pid、sessionId、cwd、kind(interactive/background)、status(busy/waiting/idle)、state(working/blocked/done/failed) |
| 生命周期钩子 | hooks，支持 `type:"command"` 与 **`type:"http"`** | 正式稳定 | SessionStart/End、UserPromptSubmit、Stop、Notification、PermissionRequest 等事件直推一个 HTTP URL，入参带 session_id/transcript_path/cwd |
| 无人值守执行 | `claude -p --output-format stream-json` | 正式文档化 CLI 契约 | 新开会话执行任务；`--input-format stream-json` 支持多轮驱动与 interrupt；事件含 init/assistant(tool_use 可见)/stream_event/result(成本、拒批) |
| 接续历史 | `-p --resume <sid>` / `--fork-session` | 正式 | 可接续交互会话的历史；但**活会话双开导致 transcript 交错写**，安全姿势是 fork（新 id、不动原件）；被 supervisor 托管的活会话被占用时管道化调用直接退出码 1 |
| 托管后台会话 | `claude --bg` + `agents/attach/logs/stop/respawn` | research preview | 官方进程托管（热备用、崩溃重启、关终端不死、可人工 attach），流式粒度不如 -p |
| 向活会话发话 | 跨会话 inbox socket（每会话 UDS，句柄经 `CLAUDE_CODE_MESSAGING_SOCKET/TOKEN` 传入） | 正式（v2.1.224+） | 注入一段**文本**，对方在工具调用间隙自行决定怎么处理；无返回通道、受 inbound 策略约束、斜杠命令不执行、不能批权限 |
| 外部事件推进活会话 | channels（MCP `claude/channel`、webhook） | research preview、隐藏 flag、白名单门控 | 外部消息作为 `<channel>` 注入并可双向——最像「Go 程序 webhook 推会话」，但现阶段不能押生产 |
| 转录文件 | `~/.claude/projects/<编码cwd>/<sid>.jsonl` | **内部格式，版本间会变**（官方原话） | 只读兜底，不作主通道 |

## 候选架构

### A 档：会话发现改走 `claude agents --json`（低风险，约 1 天）

用官方发现口替换自研心跳守护的大部分机械：

- serve 每 30–45s 跑一次 `claude agents --json`（无 TTY，`--cwd` 可过滤），
  直接得到全舰队会话与 busy/waiting 状态——**删掉每会话守护循环、pidfile、
  findAncestorPID、SessionEnd 清理**（也修掉当前 hook 沿父进程链误认 daemon pid
  导致探活对象不死的小瑕疵）。
- 钩子保留但变轻：SessionStart 做即时点卯（免去轮询延迟），claim/done 的任务号
  仍写心跳文件；可顺手把 command hook 换成 **HTTP hook 直推 127.0.0.1:8787**，
  省掉每会话一次 shell fork。
- 兜底：`agents --json` 不可用（旧版本/preview 变动）时退回现有钩子模型；
  远程 ssh 船继续走扁平心跳。

### B 档：自动开船——headless 执行派船任务（中风险，约 2–4 天）

派船到「无在线会话的船位」时，由 serve 在该目录 spawn 一个 worker：

```
claude -p --output-format stream-json --verbose --include-partial-messages \
  --permission-mode acceptEdits --allowedTools <白名单> --permission-prompts none \
  --session-id <由思港分配> "<任务单模板：show T 编号 + 验收清单 + 协作规约>"
```

- Go 侧逐行解析：assistant/tool_use → `port log` 与芯片状态行；
  验收勾选由船调 `port check`（现有 CLI 已支持）；`result` 行收尾 `port done`，
  退出码 143/1/2 区分被杀/拒批/失败 → blocked。
- 需要长期托管、人可随时 attach 的任务走 `claude --bg`（与 -p 两种船型并存）。
- 绝不碰活的交互会话：接续上下文一律 `--resume <sid> --fork-session`；
  对交互会话的派船维持现状（人在终端里 `port claim`）。
- **安全边界（硬要求）**：serve 当前绑 0.0.0.0、Tailscale 全网可达且无鉴权，
  开船 = 在 Studio 上执行代码。开船端点必须只听 127.0.0.1 或加共享密钥；
  worker 使用**独立 `ANTHROPIC_API_KEY`**（`--bare --settings`），不占订阅 OAuth；
  权限白名单按任务重要程度分级。

### C 档：双向协航（不建议近期做）

- inbox socket 给活交互会话派活：语义是「注入文本请它自己看着办」，无执行契约、
  无返回、可能被 hold，与思港「派船是命令、接令/归港有硬门禁」的模型不合。
- channels webhook 最贴合但仍是 research preview + 白名单门控。
- Remote Control 维持**给人用**：它反而是思港的好搭档——人在手机上用 PWA 派船，
  需要亲自掌舵时用官方 App remote-control 接管本机会话；两条通道不集成。

## 决策建议

1. **近期做 A 档**：会话发现以 `claude agents --json` 为主、钩子为即时事件增强，
   删掉自研探活守护。官方既已为 scheduler 提供受支持读取口，自维护 pidfile 是负债。
2. **中期做 B 档**：headless 自动开船是思港闭环的最大提效点，但作为派船的**可选船型**
   （交互会话人工接令的路径永远保留），并先补端点鉴权这一课。
3. **明确不做**：逆向 daemon socket、把 Remote Control 当 RPC、生产依赖 channels、
   解析 transcript 驱动流程（最多作离线历史的只读兜底）。
4. 一切官方集成都做**能力探测与回退**（`system/init.capabilities`、`agents --json`
   退出码），Claude Code 这层除 hooks/MCP 外版本变化密集，preview 能力随时可被拔线。

## 后果

- A 档后心跳子系统大幅瘦身，船位状态更准（busy/waiting 是官方状态而非 pid 猜测），
  但多一个对 `claude` CLI 的轮询依赖（约每 45s 一次轻量调用）。
- B 档后思港从「调度看板」变成「调度 + 无人码头」，安全模型从「只读本地文件」升级为
  「可启动执行体」，鉴权与权限白名单是上线前提而非后续优化。
- 不集成 Remote Control 意味着手机端不能直接在思港里掌舵——这是刻意的边界，
  掌舵交给官方 App，思港只做命令与验货。
