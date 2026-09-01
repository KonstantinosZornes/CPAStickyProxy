import type { AccountFilters } from "./types";

export function appliedProxyValue(filters: AccountFilters): string {
  return filters.proxy_id;
}

export function updateAppliedProxyFilter(
  filters: AccountFilters,
  value: string,
): AccountFilters {
  return { ...filters, proxy_state: "", proxy_id: value };
}

export function updateProxyStateFilter(
  filters: AccountFilters,
  value: string,
): AccountFilters {
  return { ...filters, proxy_state: value, proxy_id: "" };
}
