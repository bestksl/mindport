# CC Switch 式切换能力 — 边缘增强清单

## 背景

MindPort 已具备完整的 CC Switch 式模型/provider 切换能力，核实如下：

- **多配置档**：`ProviderInstanceConfig`（`packages/contracts/src/providerInstance.ts:75`）— 一个 instance = driver + environment（注入 `ANTHROPIC_BASE_URL`/`ANTHROPIC_AUTH_TOKEN`）+ config（`customModels` 等）。OpenRouter/网关场景见 `docs/user/providers-claude.md`。
- **全局当前档**：`ServerSettings.defaultModelSelection: NullOr(ModelSelection)`（`packages/contracts/src/settings.ts:1220`），含 project 级覆盖（`ProjectSettingsOverrides.defaultModelSelection`，`:1063`）。
- **设置 UI 切换**：`apps/web/src/components/settings/ProjectDefaultsSettings.tsx:129` — `updateSettings({ defaultModelSelection: value })`。
- **新会话读默认档**：`apps/web/src/components/chat/DraftHeroHeadline.tsx:218-245` — 新建草稿读 `defaultModelSelection`（含 project override 回退、fallback resolution）并应用。
- **运行中 per-thread 切换**：`provider.switch`（跨 instance）与 `thread.model-selection.set`（同 instance 换 model），走 `dispatchThreadMutation`（`apps/server/src/orchestration-v2/Orchestrator.ts:13119-13121`）。
- **热切换**：Claude SDK `query.set_model` 经 `apps/server/src/orchestration-v2/Adapters/ClaudeAdapterV2.ts:1165-1177` 包裹，turn 内换 model 不重启进程。
- **context handoff**：`apps/server/src/orchestration-v2/ProviderSwitchService.ts` + `docs/orchestration-v2/provider-switching-and-context.md`。
- **model picker UI**：`apps/web/src/components/chat/ProviderModelPicker.tsx`。

以下 10 项是核实后确认的**边缘增强空缺**，每项含落点与验收标准。

---

## 1. 批量切换多 thread 到当前档

**空缺**：无「一键把所有现有 thread 切到全局当前档」的命令/UI。`provider.switch` 必须 per-thread（`Orchestrator.ts:592` 带 `threadId`）。

**落点**：新增 WS 命令 `provider.switch-all`（或 client 层循环发 `provider.switch`）；UI 入口在 `ProjectDefaultsSettings.tsx` 或 sidebar。

**验收**：选中多个/全部 thread，一键切到 `defaultModelSelection`，每个 thread 走 `ProviderSwitchService.plan()`，事件流可回放。

## 2. preset / 连接模板

**空缺**：`connectionPreset`/`providerPreset`/`providerTemplate` 全仓 0 命中。用户每次新建 instance 要手填 env + customModels，无法复用「endpoint+key+模型列表」组合。

**落点**：`packages/contracts/src/settings.ts` 加 `connectionPresets: Record<PresetId, ConnectionPreset>`，`ConnectionPreset = { driver, displayName, environment, customModels }`；`AddProviderInstanceDialog` 加「从模板创建」。

**验收**：保存一套 preset，新建 instance 时一键套用，env/secret 引用正确继承。

## 3. 顶栏全局快切器

**空缺**：只有 per-thread 的 `ModelPickerSidebar`（`apps/web/src/components/chat/ModelPickerSidebar.tsx`），无顶栏全局当前档快切器。

**落点**：chat 顶栏加全局 provider/model 快切器，写 `defaultModelSelection`。

**验收**：顶栏一键切全局当前档，新 thread 立即跟随，不改动现有 thread。

## 4. 真实 model id 透出

**空缺**：`nativeModel` 仅在 `apps/web/src/modelSelection.test.ts` 出现，运行时 UI 未广泛透出网关重映射后的真实 model id。model id 经 `compileClaudeModelSelection` → `apiModelId` → `ANTHROPIC_DEFAULT_*_MODEL`/`ANTHROPIC_MODEL` 再映射（`apps/server/src/provider/claudeConfiguredModel.ts`），网关还能再改。

**落点**：读 `OrchestrationV2ThreadLatestModelReport.nativeModel`（`packages/contracts/src/orchestrationV2.ts:2075`），在 picker/状态线显示。

**验收**：切换后显示「选择: X / 实际: Y」，网关重映射时 Y 与 X 不同。

## 5. 切换历史 / audit 面板

**空缺**：`switch history`/`auditLog` 0 命中。event-sourced 已记录 `thread.provider-switched`/`thread.model-selection-updated` 事件（`orchestrationV2.ts:1890`），但无回看 UI。

**落点**：新增 settings 页读 `projection_thread_sessions` + 事件流，渲染切换时间线。

**验收**：显示某 thread 的 provider/model 切换历史，含时间、来源、前后值。

## 6. managed policy 兼容

**空缺**：`apps/server/src/policy/managedProviders.ts` 锁定 `homePath`（`:172-176`）并可禁用 provider（`use-disallowed-provider`），但「全局档切到被托管锁定的 instance」路径无明确提示与降级。

**落点**：`ProjectDefaultsSettings.tsx` 写 `defaultModelSelection` 前校验目标 instance 是否被 managed policy 锁定/禁用，锁定时禁用并提示。

**验收**：托管部署下切到禁用 instance 被拦，UI 显示策略原因。

## 7. 命名与文档规范

**空缺**：`profile`（browser profile 占用，`packages/contracts/src/browserProfile.ts`）、`preset`（`SnoozePresetSetting`/`PreviewViewportPreset` 占用）、`context`（`ContextHandoff`/`ContextTransfer` 强语义）均被占用。新功能若沿用会语义打架。

**落点**：为 preset/快切器定名（候选 `connectionPreset`/`activeConnection`），更新 `docs/user/providers-claude.md` 与 `docs/internals/adding-a-provider.md`。

**验收**：新命名不与现有三词冲突，docs 反映全局档 + preset 用法。

## 8. CLAUDE_CONFIG_DIR keychain 隔离

**空缺**：`apps/server/src/provider/Drivers/ClaudeHome.ts:31-47` 刻意不动 HOME 以保 Keychain，注释承认 Claude OAuth token 按 `CLAUDE_CONFIG_DIR` 存，但 macOS Keychain 项归属不一定按 config dir 分。同 binary 多 endpoint（N 个 instance）时账号隔离边界未明确。

**落点**：核实 + 文档化 keychain 隔离边界；若不隔离，在 `docs/user/providers-claude.md` 明确「多账号 keychain 共享」风险与用 `ANTHROPIC_AUTH_TOKEN`（非 OAuth）规避的指引。

**验收**：docs 明确多账号 keychain 行为，用户知道何时该用 token 而非 OAuth。

## 9. 全局切换确认 / 防误触

**空缺**：`updateSettings({ defaultModelSelection })`（`ProjectDefaultsSettings.tsx:129`）无确认。改全局当前档影响所有新 thread，误触成本低。

**落点**：全局档变更加确认对话框（区别于 per-thread 切换），说明影响范围。

**验收**：切全局档时弹确认，显示「将影响所有新建会话」，per-thread 切换不弹。

## 10. 当前档状态全局可见

**空缺**：无顶栏/状态线显示当前生效的 provider+model+endpoint。当前档只在 Settings 页可见，chat 顶栏只显示 per-thread 选择。

**落点**：顶栏或状态线读 `defaultModelSelection` + 对应 instance 的 endpoint，常驻显示。

**验收**：顶栏常驻显示「全局: Claude / opus-5-5 / api.anthropic.com」，切换后实时更新。

---

## 核实方法

每项空缺均经 grep 核实（`apps/` + `packages/` + `packages/contracts/src`，`.ts`/`.tsx`，排除 `.test.`）。落点 file:line 基于 commit `8b0ca6d51`。
