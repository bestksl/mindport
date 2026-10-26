# 远程使用指南：在公司连家里的 Studio

适用场景：Mac Studio 在家常开，人在公司，**Android 手机和笔记本都要从外网访问**
思港 Web 与 Studio 命令行。方案：**Tailscale 组网**（WireGuard 加密虚拟内网，
免费，免公网 IP、免路由器端口转发、免域名）。

```
公司笔记本 ─┐
Vs3 手机 ──┼── Tailscale tailnet（任何网络下如同同一局域网）
家里 Studio ┘
   ├─ 手机 Chrome  → http://<studio-tailscale>:8787   思港 Web/PWA
   └─ 笔记本 ssh   → ssh jmzx@<studio-tailscale>       完整命令行（含 port）
```

> Tailscale 装上后，设备间直连优先（NAT 打洞），不经过任何服务器；
> 打洞失败时才走加密中继。Web 和 SSH 是同一个通道，**不需要单独做内网穿透**。

---

## 一、Studio 端（一次性，在家操作）

1. 安装 Tailscale（安装包已下载到 `~/Downloads/Tailscale-*-macos.pkg`，双击安装）；
   也可从 https://pkgs.tailscale.com/stable/ 或 Mac App Store 安装。
2. 打开 Tailscale，Log in（用 GitHub/Google/微软账号均可，国内可正常完成授权），
   记下这台机器在 tailnet 里的名字（菜单栏图标 → 可看到 100.x.x.x 地址与机器名）。
3. 开启「远程登录」（系统 SSH）。二选一：
   - 系统设置 → 通用 → 共享 → 远程登录 → 打开；或
   - 终端执行：`sudo systemsetup -setremotelogin on`
4. 确认思港服务在跑（监听 0.0.0.0 即可被 tailnet 访问）：
   ```bash
   port serve --host 0.0.0.0 --port 8787
   # 已在跑则无需重启；验证：curl http://100.x.x.x:8787/  （Studio 的 Tailscale 地址）
   ```

CLI 路径（不依赖菜单栏 App）：

```bash
/Applications/Tailscale.app/Contents/MacOS/Tailscale status   # 看 tailnet 成员与 IP
/Applications/Tailscale.app/Contents/MacOS/Tailscale ip -4    # 本机 100.x 地址
```

## 二、Vs3 手机端（一次性）

荣耀无 Google 服务，直接装 F-Droid 版 APK：

1. 浏览器打开 https://f-droid.org/packages/com.tailscale.ipn/ 下载安装
   （第一次需允许「安装未知来源应用」）；
2. 打开 Tailscale，用**同一个账号**登录，拨动开关连接；
3. Chrome 访问 `http://<studio-tailscale-ip>:8787`（如 `http://100.64.1.5:8787`），
   菜单「添加到主屏幕」即得到思港图标；
4. 出门后保持 Tailscale 连接即可，用法和在家 WiFi 完全一样。

若想在手机上直接敲命令行：装 **Termius**（SSH 客户端），host 填 Tailscale 地址，
账号 `jmzx`，密钥见第三节。

## 三、笔记本端（一次性）

1. 安装 Tailscale（https://tailscale.com/download ，macOS/Windows/Linux 都有），
   同一账号登录并连接；
2. 免密登录（推荐密钥；macOS/Linux，Windows 用 PowerShell 同理）：
   ```bash
   ssh-keygen -t ed25519                      # 已有密钥可跳过
   ssh-copy-id jmzx@<studio-tailscale-ip>     # 输一次 Mac 登录密码
   ssh jmzx@<studio-tailscale-ip>             # 以后免密
   ```
3. `~/.ssh/config` 起个别名，以后 `ssh studio` 即可：
   ```
   Host studio
       HostName <studio-tailscale-ip>
       User jmzx
       ServerAliveInterval 30
   ```
   若启用了 MagicDNS（tailnet 设置里默认开启），HostName 可直接写 Studio 的
   tailnet 机器名，IP 变化也不用改配置。
4. 登录后即可远程使用：
   ```bash
   ssh studio
   port list
   port claim T0003
   tail -f ~/MindPort/.port/serve.log
   ```

## 四、验收清单

- [ ] 公司网络下，手机 Tailscale 显示 connected，浏览器打开思港页面 200
- [ ] 手机入港一条念头，Studio 上 `port list` 的草稿区立即出现新 T 编号
- [ ] 笔记本 `ssh studio` 登录成功，`port list` 正常
- [ ] Studio 重启后：先启动 Tailscale（设为开机自启），再启动 `port serve`

## 五、安全说明

- Tailscale 是**私有网络**：只有你账号下、显式登录的设备能看到 Studio，
  公网扫描不到 8787 与 22 端口；
- SSH 登录仍受 macOS 账户密码/密钥保护；建议关闭密码登录、只留密钥
  （`/etc/ssh/sshd_config` 中 `PasswordAuthentication no`，熟了再改）；
- 思港 v0 的 Web **没有应用层鉴权**——这是有意为之：它只在家庭 LAN 与你的
  tailnet 内可达。不要把 8787 端口通过路由器转发到公网。

## 六、保底方案（若 Tailscale 在你的网络下不可用）

国内少数网络对 UDP 不友好会导致直连失败、中继偏慢，备选：

1. **Tailscale 中国版**（cn.tailscale.com，国内伙伴运营的独立网络，需另注册，
   与国际版网络不互通）；
2. **frp / rathole + 一台云服务器**：自建 TCP 隧道，SSH 与 HTTP 都能转发，
   最可控但需要 VPS；
3. ZeroTier、蒲公英等同类组网产品。

遇到连通问题先看 `Tailscale status` 中 Studio 的 IP 后是 `direct` 还是 `relay`，
把结果带来，按情况选方案。
