# ADR-0002：实现语言选择 Go

- 状态：**Accepted**
- 日期：2026-10-25
- 相关：[ADR-0001](0001-local-first-single-machine.md)

## 背景

思港的运行形态由 ADR-0001 确定：全部计算发生在一台常开的 Mac Studio 上，一个程序同时承担 CLI、本机 HTTP daemon、内嵌 Web 界面，未来可能内置 SSH 服务。头号高频用户是 Claude Code / Codex 这类 agent——它们以 shell out 方式高频调用 CLI。候选语言：Go、Rust、Python。

## 决策

采用 **Go**（首个版本以最新稳定版为准）。

## 理由

1. **单静态二进制分发**：最终产物只有一个 `port` 可执行文件，无运行时、无 venv、无动态库；Studio 与未来的笔记本上下载即用，`brew install` 友好。
2. **agent 冷启动成本为零**：编译型二进制毫秒级启动，agent 每次 `port list --json` 不付解释器初始化代价；Python 的 50–100ms 启动税在高频 shell out 下会被持续感知。
3. **三个子系统都是一等公民**：标准库 `net/http` 即生产级；`golang.org/x/crypto/ssh` 是众多工具内置 SSH 的同款实现；`go:embed` 可把 PWA 静态资源打进同一个二进制，CLI 与 daemon 不分包。
4. **开发速度适配 pre-alpha**：语法简单、决策面小，文件 I/O、YAML、CLI（cobra）类任务是 Go 的舒适区，迭代吞吐高。
5. **交叉编译一行完成**：`GOOS=darwin GOARCH=arm64 go build`，未来加机器（含 Linux 服务器）零额外工具链。

## 备选方案与放弃原因

- **Rust**：单二进制与性能同样满足，但「大量小文件 I/O + 配置编排 + HTTP CRUD」用不满其性能与内存安全优势，借用检查与异步生命周期的心智税会拖慢每个迭代；等项目里真出现性能敏感组件（如索引引擎）再以独立模块评估。
- **Python**：原型最快，但解释器冷启动、venv/uv 环境依赖、长期 daemon 的工程脆弱性、打包分发（尤其给非本机环境）均为持续付息项；与「agent 高频调用的小工具」定位冲突。

## 后果

- 依赖从简：CLI 用 `cobra`，YAML frontmatter 用成熟 yaml 库，HTTP 先走标准库（路由器在 Go 1.22+ 的 ServeMux 已够用），前端零构建（服务端渲染 + 少量 JS/htmx）。
- Web 资源以 `go:embed` 内嵌；版本号、提交哈希通过 `-ldflags` 注入。
- 测试以标准 `testing` 为主，文件锁/状态机等核心逻辑必须有表驱动单测；本机无 CI 之外的工具链约束（CI 配置随骨架一并建立）。
