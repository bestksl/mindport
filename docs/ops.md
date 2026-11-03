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

- 监听 `0.0.0.0:8787`（局域网 + 组网地址），启动横幅分别打印两类地址；
- 日志：`~/MindPort/.port/serve.log`；
- 已知缺口：开机/登录不会自启，进程崩溃不会自动拉起；
  机器重启后需手动执行上面一行（后续上 Docker 解决）。

### 访问密钥与设备配对（Web 鉴权）

**默认不启用，内网免登录敞开**（组网/家庭内网单用户模型，2026-10-27 拍板）。
会话页会展示 transcript（含命令/代码/思考），不可信网络（公共 WiFi、
要把端口暴露出去等）用 `--auth` 上锁：

```bash
port serve --host 0.0.0.0 --port 8787 --auth   # 或 PORT_AUTH=1
```

- 上锁后：浏览器首次打开任意页 302 到 `/login`；推荐电脑运行 `port pair`
  生成 10 分钟一次性配对码，终端直接显示二维码，手机扫码即完成该设备配对。
  每台设备持有独立的 30 天滑动会话（mp_sid cookie，HttpOnly、SameSite=Lax），
  `/api/*` 未登录直接 401；顶栏出现登出口。
- 设备管理：`port devices` 查看已配对设备（设备名取自 UA、带来源 IP 与
  最近活跃时间），`port devices revoke <id前缀>` 单独吊销一台，不影响其他设备。
- 兜底：`~/MindPort/.port/state/web.token` 是 32B 随机 hex 共享密钥（0600），
  `port web-token` 查看/首建，在 `/login` 粘贴后同样签发设备会话
  （浏览器不再保存密钥本体）；旧版直接把密钥写进 mp_key cookie 的方式仍兼容。
- 配对码只存 SHA-256 哈希、10 分钟过期、用一次即作废，经 URL fragment 传递
  （不进服务端日志）；登录/配对按 IP 限频（每分钟 10 次，本机豁免）。
- 手机换机/丢 cookie：在本机重新 `port pair` 即可；旧设备可在 `port devices` 吊销。
- ⚠ 无鉴权 + `--host 0.0.0.0` 时，同 WiFi 网段任何人都能读全部内容，
  启动横幅会明示这一警告；组网网段（如蒲公英 172.16/12 等，
  未开公网中继）只有自己的设备，日常默认模式即依赖这道网络边界，不做用户体系。

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

## 在线状态（运行时数据）

在线状态文件在 `~/MindPort/.port/state/agents/`：v0.4 起按
`<actor>/<session>.json` 每个项目会话一条（含 dir/name/kind/task/note/seen），
旧版扁平 `<actor>.json` 仍兼容读取（同一助手已有会话记录时看板自动折叠，
避免旧钩子残留造成重复芯片）。属于运行时状态、不进 git 同步；20 秒防抖。
v0.5 起**不再有探活守护循环与 pidfile**：会话生死由 serve 每 30 秒执行
`claude agents --json` 的活会话全集判定（见 [ADR-0007](adr/0007-agent-integration.md)
A 档），发现口缺席时退回 2 分钟 TTL。升级到 v0.5 后若残留 v0.4 的旧守护，
`pkill -f 'heartbeat.*--loop'` 并清掉 `agents/*.hb.pid` 即可。详见
[ADR-0004](adr/0004-presence-and-publish.md)、ADR-0007 与 agent-flow.md「在线状态」。

## 会话页与留言（ADR-0008）

- 时间线是对 `~/.claude/projects/<编码cwd>/<sid>.jsonl` 的**只读**桥接：
  按字节偏移增量拉取（1.5s 轮询，页面隐藏暂停），首屏读最后 128KB，
  上翻 128KB/块，半行不解析、游标回退行首，uuid 去重——不丢不重、可全量滚动；
  思考与工具原文默认折叠。思港不 attach、不模拟键盘、不改写 transcript。
- 留言：空闲/离线会话走 `claude -p <msg> --resume <sid> --bg` 后台续跑；
  会话正忙（官方拒绝并发 resume）时留言落 `~/MindPort/.port/state/mail/<sid>.jsonl`，
  UserPromptSubmit 钩子 `port mail-pull` 在下一轮以 `additionalContext` 幂等注入。
- 仅 Claude Code 会话成立（Codex 无 transcript）。

## 远程访问

见 [remote.md](remote.md)。组网地址连不通时，先在组网客户端里确认 Studio
在线且组网 IP 与 `port serve` 横幅上的**组网地址**一致；若机器上跑着代理软件，
检查它的 fake-ip 是否劫持了组网网段，必要时给该网段加 DIRECT 规则。
