# 运维：生产服务、测试隔离、升级

## 测试 / 生产分离

| | 生产 | 本地测试 |
|---|---|---|
| vault | `~/MindPort`（真实念头与任务） | 临时目录，如 `mktemp -d`（旧脚本用 `/tmp/port-v02`） |
| 端口 | 8787 | 高位临时端口（如 18790） |
| 进程 | launchd 托管（见下） | 手动 `./bin/port serve`，测完即杀 |
| 数据 | 永远不要拿生产 vault 做冒烟 | 测完整目录删除，不影响生产 |

冒烟测试一律显式指定 vault 与端口，不碰默认值：

```bash
T=$(mktemp -d)
PORT_VAULT=$T ./bin/port init
PORT_VAULT=$T ./bin/port serve --host 127.0.0.1 --port 18790
```

## 生产服务（macOS launchd）

配置模板在 [`deploy/com.bestksl.mindport.plist`](../deploy/com.bestksl.mindport.plist)：

- 登录自启（`RunAtLoad`）；
- 崩溃非 0 退出 → 10 秒后自动拉起（`KeepAlive` + `ThrottleInterval`）；
- 正常 SIGTERM 退出**不**自动重启，便于升级窗口；
- 监听 `0.0.0.0:8787`（局域网 + Tailscale），vault 固定 `~/MindPort`；
- stdout/stderr 都落盘到 `~/MindPort/.port/serve.log`。

安装（一次性，需本人执行）：

```bash
go build -o ~/.local/bin/port ./cmd/port        # 注意：先 rm 旧二进制再 cp，
codesign --force -s - ~/.local/bin/port         # 否则原地覆盖会导致签名失效被内核杀掉（137）
cp deploy/com.bestksl.mindport.plist ~/Library/LaunchAgents/
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.bestksl.mindport
```

日常操作：

```bash
launchctl print gui/$(id -u)/com.bestksl.mindport   # 状态
launchctl kickstart -k gui/$(id -u)/com.bestksl.mindport  # 重启（升级二进制后）
launchctl kill SIGTERM gui/$(id -u)/com.bestksl.mindport  # 优雅停服
tail -f ~/MindPort/.port/serve.log                 # 看日志
```

## 升级流程

1. 新版本先在临时 vault + 临时端口冒烟；
2. `rm ~/.local/bin/port && cp 新二进制 ~/.local/bin/port && codesign --force -s - ~/.local/bin/port`；
3. launchd 托管时 `launchctl kickstart -k ...`；临时 nohup 时 `kill <旧PID>` 后重新 nohup 启动。

serve 支持 SIGTERM/SIGINT 优雅退出（最多等 10s 在途请求），并设置了
`ReadHeaderTimeout`（10s）与 `IdleTimeout`（120s）；`WriteTimeout`
故意为 0——SSE（`/events`）是长连接流式响应。

## 远程访问

见 [remote.md](remote.md)。Tailscale MagicDNS 域名走不通时，通常是代理软件的
fake-ip 把 `*.ts.net` / `100.64.0.0/10` 劫持了：给代理加两条 DIRECT 规则，
或直接用 Tailscale IP `http://100.x.y.z:8787`。
