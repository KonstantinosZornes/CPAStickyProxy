import type { AccountProxyState } from "./types";

export type ProxyStateCategory =
  | "sticky_applied"
  | "unconfigured_proxy"
  | "custom_other"
  | "proxy_attention";

const CATEGORY_STATES: Record<
  ProxyStateCategory,
  readonly AccountProxyState[]
> = {
  sticky_applied: ["sticky_applied"],
  unconfigured_proxy: ["inherits_system"],
  custom_other: ["custom_other"],
  proxy_attention: ["invalid_proxy", "unreadable"],
};

export const PROXY_STATE_CATEGORIES: readonly ProxyStateCategory[] = [
  "sticky_applied",
  "unconfigured_proxy",
  "custom_other",
  "proxy_attention",
];

export function proxyStateFilterValue(category: ProxyStateCategory): string {
  return CATEGORY_STATES[category].join(",");
}

export function proxyStateCategory(
  state: AccountProxyState,
): ProxyStateCategory | undefined {
  return PROXY_STATE_CATEGORIES.find((category) =>
    CATEGORY_STATES[category].includes(state),
  );
}

export function proxyStateSelectValue(value: string): string {
  const selected = new Set(
    value
      .split(",")
      .map((state) => state.trim())
      .filter(Boolean),
  );
  for (const category of PROXY_STATE_CATEGORIES) {
    const states = CATEGORY_STATES[category];
    if (
      states.every((state) => selected.has(state)) &&
      selected.size === states.length
    ) {
      return proxyStateFilterValue(category);
    }
  }
  return value;
}
