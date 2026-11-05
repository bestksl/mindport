import { describe, expect, it } from "vite-plus/test";
import { resolveClaudeConfiguredModel } from "./claudeConfiguredModel.ts";

describe("resolveClaudeConfiguredModel", () => {
  it("is undefined without any model environment override", () => {
    expect(
      resolveClaudeConfiguredModel({ selectedModel: "claude-opus-5-5", env: { PATH: "/bin" } }),
    ).toBeUndefined();
  });

  it("maps an opus selection through the opus alias variable", () => {
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-opus-5-5",
        env: { ANTHROPIC_DEFAULT_OPUS_MODEL: "ark-code-latest" },
      }),
    ).toBe("ark-code-latest");
  });

  it("picks sonnet and haiku family variables", () => {
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-sonnet-5-5",
        env: { ANTHROPIC_DEFAULT_SONNET_MODEL: "sonnet-alias" },
      }),
    ).toBe("sonnet-alias");
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-haiku-5-5",
        env: { ANTHROPIC_DEFAULT_HAIKU_MODEL: "haiku-alias" },
      }),
    ).toBe("haiku-alias");
  });

  it("falls back to ANTHROPIC_MODEL for families without a dedicated variable", () => {
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-fable-5-1",
        env: { ANTHROPIC_MODEL: "ark-code-latest" },
      }),
    ).toBe("ark-code-latest");
  });

  it("prefers the family alias over the generic model variable", () => {
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-opus-5-5",
        env: {
          ANTHROPIC_DEFAULT_OPUS_MODEL: "opus-alias",
          ANTHROPIC_MODEL: "generic-alias",
        },
      }),
    ).toBe("opus-alias");
  });

  it("uses the generic model variable when the family variable is empty", () => {
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-opus-5-5",
        env: { ANTHROPIC_DEFAULT_OPUS_MODEL: "  ", ANTHROPIC_MODEL: "ark-code-latest" },
      }),
    ).toBe("ark-code-latest");
  });

  it("is undefined when the override just repeats the selected slug", () => {
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-opus-5-5",
        env: { ANTHROPIC_DEFAULT_OPUS_MODEL: "CLAUDE-OPUS-5-5" },
      }),
    ).toBeUndefined();
  });

  it("trims surrounding whitespace from the configured id", () => {
    expect(
      resolveClaudeConfiguredModel({
        selectedModel: "claude-opus-5-5",
        env: { ANTHROPIC_DEFAULT_OPUS_MODEL: "  ark-code-latest  " },
      }),
    ).toBe("ark-code-latest");
  });
});
