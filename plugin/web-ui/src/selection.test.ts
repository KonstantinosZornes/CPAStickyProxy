import { describe, expect, it } from "vitest";
import {
  emptySelection,
  invertSelection,
  selectAllFiltered,
  selectionPayload,
  toggleSelection,
} from "./selection";
import type { AccountFilters } from "./types";

const filters: AccountFilters = {
  q: "",
  provider: "codex",
  type: "oauth",
  status: "active",
  proxy_state: "",
  proxy_id: "",
  include_disabled: "false",
  priority: "",
  note: "",
};

describe("account selection", () => {
  it("keeps an explicit selection compact and toggleable", () => {
    const selected = toggleSelection(emptySelection(), "auth-a");
    expect(selectionPayload(selected)).toEqual({ auth_indexes: ["auth-a"] });
    expect(toggleSelection(selected, "auth-a")).toEqual({
      kind: "explicit",
      authIndexes: new Set(),
    });
  });

  it("inverts compact selections within the current filter", () => {
    const explicit = toggleSelection(emptySelection(), "auth-a");
    expect(invertSelection(explicit, filters)).toEqual({
      kind: "filtered",
      filters,
      excludedAuthIndexes: new Set(["auth-a"]),
    });

    const filtered = toggleSelection(selectAllFiltered(filters), "auth-b");
    expect(invertSelection(filtered, filters)).toEqual({
      kind: "explicit",
      authIndexes: new Set(["auth-b"]),
    });
  });

  it("sends filter snapshot and exclusions instead of all matching IDs", () => {
    const all = selectAllFiltered(filters);
    const excluded = toggleSelection(all, "auth-b");
    expect(selectionPayload(excluded)).toEqual({
      selection: {
        kind: "filtered",
        filters,
        excluded_auth_indexes: ["auth-b"],
      },
    });
    expect(toggleSelection(excluded, "auth-b")).toMatchObject({
      kind: "filtered",
      excludedAuthIndexes: new Set(),
    });
  });
});
