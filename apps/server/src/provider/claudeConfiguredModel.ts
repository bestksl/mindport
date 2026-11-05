/**
 * The model id Claude Code's own configuration maps a catalog selection to before the request
 * leaves the machine (ANTHROPIC_DEFAULT_*_MODEL aliases, then ANTHROPIC_MODEL). A gateway can
 * remap that id again server-side; the provider-reported native id is captured separately.
 */
export function resolveClaudeConfiguredModel(input: {
  readonly selectedModel: string;
  // Effective environment: Claude settings' env block over the process environment.
  readonly env: Readonly<Record<string, unknown>>;
}): string | undefined {
  const family = /opus/i.test(input.selectedModel)
    ? "ANTHROPIC_DEFAULT_OPUS_MODEL"
    : /sonnet/i.test(input.selectedModel)
      ? "ANTHROPIC_DEFAULT_SONNET_MODEL"
      : /haiku/i.test(input.selectedModel)
        ? "ANTHROPIC_DEFAULT_HAIKU_MODEL"
        : undefined;
  const candidates = [
    ...(family === undefined ? [] : [input.env[family]]),
    input.env.ANTHROPIC_MODEL,
  ];
  const configured = candidates
    .find((value): value is string => typeof value === "string" && value.trim().length > 0)
    ?.trim();
  if (configured === undefined) return undefined;
  return configured.toLowerCase() === input.selectedModel.toLowerCase() ? undefined : configured;
}
