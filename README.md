# MindPort

手机浏览器里的 agent 遥控台：在电脑上运行一个 Node 服务，手机或任意浏览器通过网页对接本机的 Claude Code、Codex 等命令行 agent——远程发起会话、手机端审批权限、实时查看思考链与工具执行，无需安装原生 App。

## 特性

- **单服务 Web 架构**：Node WebSocket 服务直接托管网页端，局域网或内网穿透即可访问，移动端 390px 视口专项调校
- **四种权限模式**：Supervised（逐条审批）/ Auto-accept edits / Auto / Full access，危险操作可在手机上即时批准或拒绝
- **会话管理**：fork 分叉、settle / snooze 标记、重命名、归档；事件溯源 + SQLite 持久化，刷新与重连不丢状态
- **多 provider 适配层**：Claude Code、Codex、Cursor 等命令行 agent 统一接入
- **思考链可视化**：意图、工具调用、命令与结果分层折叠展示，模型真实标签与供应商标识如实显示
- **配对码登录**：一次性配对码兑换会话，默认 30 天有效，无需预置账号密码

## 从源码运行

要求 Node 26+ 与 pnpm 12+。

```bash
pnpm install --frozen-lockfile
pnpm exec vp run --filter @t3tools/web build
node apps/server/src/bin.ts serve --lan --port 8790 --base-dir ./.t3-live --no-browser
```

服务启动后终端会打印连接地址、一次性配对码、Pairing URL 与二维码。手机打开 Pairing URL 即自动完成配对；也可以浏览器打开 `http://<电脑地址>:8790/pair`，手动粘贴配对码登录。

随时另行签发配对码：

```bash
node apps/server/src/bin.ts pair --base-dir ./.t3-live --ttl 24h --label my-phone
```

## 目录

- `apps/server` — WebSocket 服务、编排、provider 适配、鉴权
- `apps/web` — React / Vite 网页端
- `apps/desktop` / `apps/mobile` — Electron 桌面端与 React Native 移动端
- `packages/` — 跨端契约、共享运行时与工具库
- `docs/user/` — 使用文档；`docs/internals/` — 架构与工程约定

工程规范与提交流程见 [AGENTS.md](./AGENTS.md) 与 [CONTRIBUTING.md](./CONTRIBUTING.md)。

## 许可

MIT，见 [LICENSE](./LICENSE)。
