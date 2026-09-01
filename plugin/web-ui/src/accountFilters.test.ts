import { describe, expect, it } from "vitest";
import { appliedProxyValue, updateAppliedProxyFilter } from "./accountFilters";
import type { AccountFilters } from "./types";

const filters: AccountFilters = {
  q: "",
  provider: "",
  type: "",
  status: "",
  proxy_state: "",
  proxy_id: "proxy-1",
  include_disabled: "",
  priority: "",
  note: "",
};

describe("current applied proxy filter", () => {
  it("filters by the selected proxy and clears a proxy-state filter", () => {
    const next = updateAppliedProxyFilter(
      { ...filters, proxy_state: "inherits_system" },
      "proxy-2",
    );

    expect(next).toMatchObject({
      proxy_state: "",
      proxy_id: "proxy-2",
    });
    expect(appliedProxyValue(next)).toBe("proxy-2");
  });
});
