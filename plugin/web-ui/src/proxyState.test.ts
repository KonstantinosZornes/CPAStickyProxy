import { describe, expect, it } from "vitest";
import {
  proxyStateCategory,
  proxyStateFilterValue,
  proxyStateSelectValue,
} from "./proxyState";

describe("simplified proxy-state categories", () => {
  it("groups invalid and unreadable configurations into one filter", () => {
    expect(proxyStateFilterValue("proxy_attention")).toBe(
      "invalid_proxy,unreadable",
    );
    expect(proxyStateSelectValue("invalid_proxy,unreadable")).toBe(
      "invalid_proxy,unreadable",
    );
    expect(proxyStateCategory("invalid_proxy")).toBe("proxy_attention");
    expect(proxyStateCategory("unreadable")).toBe("proxy_attention");
  });

  it("maps an empty account proxy to the unconfigured category", () => {
    expect(proxyStateCategory("inherits_system")).toBe("unconfigured_proxy");
    expect(proxyStateSelectValue("inherits_system")).toBe("inherits_system");
  });

  it("does not classify account availability states as proxy states", () => {
    expect(proxyStateCategory("no_email")).toBeUndefined();
    expect(proxyStateCategory("runtime_only")).toBeUndefined();
  });
});
