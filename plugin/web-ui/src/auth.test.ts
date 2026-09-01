import { describe, expect, it } from "vitest";
import { normalizeLanguage } from "./auth";

describe("CPA language normalization", () => {
  it("uses Chinese only for zh language tags", () => {
    expect(normalizeLanguage("zh-CN")).toBe("zh");
    expect(normalizeLanguage("zh-TW")).toBe("zh");
  });

  it("defaults unsupported, empty, and English tags to English", () => {
    expect(normalizeLanguage("en-US")).toBe("en");
    expect(normalizeLanguage("ja-JP")).toBe("en");
    expect(normalizeLanguage("fr-FR")).toBe("en");
    expect(normalizeLanguage("")).toBe("en");
    expect(normalizeLanguage(undefined)).toBe("en");
  });
});
