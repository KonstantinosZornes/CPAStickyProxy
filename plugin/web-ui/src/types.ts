export type Language = "zh" | "en";

export type ProxyPlatform =
  | "decodo"
  | "dataimpulse"
  | "resin"
  | "1024proxy"
  | "generic";

export type ProxyItem = {
  id: string;
  name: string;
  platform: ProxyPlatform;
  host: string;
  scheme: string;
  masked_base_url: string;
};

export type ProxyStateResponse = {
  ok: true;
  proxies: ProxyItem[];
};

export type AccountProxyState =
  | "sticky_applied"
  | "inherits_system"
  | "custom_other"
  | "no_email"
  | "runtime_only"
  | "invalid_proxy"
  | "unreadable";

export type AccountItem = {
  auth_index: string;
  name: string;
  email: string;
  label: string;
  provider: string;
  type: string;
  account_type: string;
  status: string;
  disabled: boolean;
  unavailable: boolean;
  priority: number;
  note: string;
  runtime_only: boolean;
  selectable: boolean;
  proxy_state: AccountProxyState;
  proxy_id?: string;
  proxy_name?: string;
  masked_proxy_url?: string;
};

export type AccountSummary = Partial<Record<AccountProxyState, number>>;

export type AccountFacets = {
  providers: string[];
  types: string[];
  statuses: string[];
  priorities: number[];
};

export type AccountPage = {
  ok: true;
  items: AccountItem[];
  total?: number;
  total_known: boolean;
  page: number;
  page_size: number;
  has_more: boolean;
  next_cursor?: string;
  summary: AccountSummary;
  facets: AccountFacets;
};

export type AccountFilters = {
  q: string;
  provider: string;
  type: string;
  status: string;
  proxy_state: string;
  proxy_id: string;
  include_disabled: string;
  priority: string;
  priority_min?: string;
  priority_max?: string;
  note: string;
};

export type FilteredSelection = {
  kind: "filtered";
  filters: AccountFilters;
  excludedAuthIndexes: ReadonlySet<string>;
};

export type AccountSelection =
  | { kind: "explicit"; authIndexes: ReadonlySet<string> }
  | FilteredSelection;

export type SyncResult = {
  mode: "apply" | "clear";
  selected: number;
  eligible: number;
  scanned: number;
  updated: number;
  already_applied: number;
  skipped: number;
  port_sticky_only: number;
  errors: Array<{
    auth_index: string;
    code: string;
  }>;
};

export type ApiFailure = {
  ok: false;
  code?: string;
  sync?: SyncResult;
};

export type AuthContext = {
  apiBase: string;
  managementKey: string;
};

export type TestResult = {
  platform: ProxyPlatform;
  ip: string;
  country: string;
  country_code: string;
  region: string;
  city: string;
  port_sticky_only: boolean;
};

export type Toast = {
  message: string;
  error?: boolean;
};
