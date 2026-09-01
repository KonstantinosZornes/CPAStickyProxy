import { describe, expect, it } from "vitest";
import { t } from "./i18n";

describe("account proxy state labels", () => {
  it("keeps the inherits_system label for accounts without an account proxy", () => {
    expect(t("inherits_system", "zh")).toBe("继承系统代理");
    expect(t("inherits_system", "en")).toBe("Inherits system proxy");
  });
});
