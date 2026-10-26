# 运维：生产服务、测试隔离、升级

## 测试 / 生产分离

| | 生产 | 本地测试 |
|---|---|---|
| vault | `~/MindPort`（真实念头与任务） | `mktemp -d` 临时目录，测完即删 |
| 端口 | 8787 | 高位临时端口（18xxx） |
| 进程 | nohup 后台（当前；后续演进 Docker） | 手动 `./bin/port serve`，测完即杀 |
| 数据 | 永远不要拿生产 vault 做冒烟 | 整目录删除，不影响生产 |

冒烟测试一律显式指定 vault 与端口，不碰默认值：

```bash
T=$(mktemp -d)
PORT_VAULT=$T ./bin/port init
PORT_VAULT=$T ./bin/port serve --host 127.0.0.1 --port 18790
```

## 生产服务（当前：nohup，保持简单）

用户已拍板：Docker 化之前先保持最简单的 nohup，不引入 launchd。

```bash
nohup env PORT_VAULT=$HOME/MindPort $HOME/.local/bin/port serve \
  --host 0.0.0.0 --port 8787 \
  >> $HOME/MindPort/.port/serve.log 2>&1 &
```

- 监听 `0.0.0.0:8787`（局域网 + Tailscale）；
- 日志：`~/MindPort/.port/serve.log`；
- 已知缺口：开机/登录不会自启，进程崩溃不会自动拉起；
  机器重启后需手动执行上面一行（后续上 Docker 解决）；
- 局域网同 WiFi 设备当前零鉴权可达，Tailscale 网段之外不可达
  （Tailscale 100.64/10 地址公网不可路由，未开 Funnel）。

`deploy/com.bestksl.mindport.plist` 是**存档备用**的 launchd 方案
（自启 + 崩溃拉起），等需要时再按文件头注释安装；当前不要启用。

## 升级流程

1. 新版本先在临时 vault + 临时端口冒烟（见上）；
2. **必须** `rm ~/.local/bin/port` 再 `cp 新二进制 ~/.local/bin/port`，
   然后 `codesign --force -s - ~/.local/bin/port`——原地覆盖已签名的
   Mach-O 会导致内核拒载，进程一启动就被 SIGKILL（exit 137）；
3. `kill -TERM <旧PID>`，等其优雅退出后重新 nohup 启动。

serve 支持 SIGTERM/SIGINT 优雅退出（最多等 10s 在途请求），并设置了
`ReadHeaderTimeout`（10s）与 `IdleTimeout`（120s）；`WriteTimeout`
故意为 0——SSE（`/events`）是长连接流式响应。

## 在线心跳（运行时状态）

心跳文件在 `~/MindPort/.port/state/agents/*.json`（守护循环 pidfile 为
`*.hb.pid`），属于运行时状态、不进 git 同步；TTL 2 分钟，20 秒防抖，
另有 `port heartbeat` 每 45 秒保活的后台循环（pgrep 探活 CLI 主进程，
退出自动 away，无需 launchd/cron）。详见
[ADR-0004](adr/0004-presence-and-publish.md) 与 README「在线心跳机制」。

## 远程访问

见 [remote.md](remote.md)。Tailscale MagicDNS 域名走不通时，通常是代理软件的
fake-ip 把 `*.ts.net` / `100.64.0.0/10` 劫持了：给代理加两条 DIRECT 规则，
或直接用 Tailscale IP `http://100.x.y.z:8787`。
