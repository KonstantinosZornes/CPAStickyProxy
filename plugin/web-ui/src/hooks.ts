import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AccountFilters, AccountPage, ProxyStateResponse } from "./types";
import type { ApiClient as Client } from "./apiClient";

const EMPTY_FILTERS: AccountFilters = {
  q: "",
  provider: "",
  type: "",
  status: "",
  proxy_state: "",
  proxy_id: "",
  include_disabled: "",
  priority: "",
  priority_min: "",
  priority_max: "",
  note: "",
};

const EMPTY_ACCOUNTS: AccountPage = {
  ok: true,
  items: [],
  total: 0,
  total_known: true,
  page: 1,
  page_size: 50,
  has_more: false,
  summary: {},
  facets: {
    providers: [],
    types: [],
    statuses: [],
    priorities: [],
  },
};

export function useProxyState(api: Client) {
  const [state, setState] = useState<ProxyStateResponse>({
    ok: true,
    proxies: [],
  });
  const [loading, setLoading] = useState(true);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      setState(await api<ProxyStateResponse>("/state"));
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    void reload().catch(() => undefined);
  }, [reload]);

  return { state, loading, reload };
}

export function useAccounts(api: Client) {
  const [data, setData] = useState<AccountPage>(EMPTY_ACCOUNTS);
  const [filters, setFilters] = useState<AccountFilters>(EMPTY_FILTERS);
  const [loading, setLoading] = useState(false);
  const cursors = useRef<Record<number, string>>({});
  const scope = useRef("");
  const request = useRef(0);
  const latestData = useRef<AccountPage>(EMPTY_ACCOUNTS);

  const query = useMemo(() => {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(filters)) {
      if (value) {
        params.set(key, value);
      }
    }
    return params;
  }, [filters]);

  const load = useCallback(
    async (page = 1) => {
      const params = new URLSearchParams(query);
      const nextScope = params.toString();
      const isFirstPage = page <= 1 || scope.current !== nextScope;
      const requiresCursor = Boolean(
        filters.proxy_state || filters.proxy_id,
      );
      if (isFirstPage) {
        page = 1;
        scope.current = nextScope;
        cursors.current = {};
      } else if (requiresCursor) {
        const cursor = cursors.current[page];
        if (!cursor) {
          return latestData.current;
        }
        params.set("cursor", cursor);
      }
      params.set("page", String(page));
      params.set("page_size", "50");
      const requestID = ++request.current;
      setLoading(true);
      try {
        const response = await api<AccountPage>(`/accounts?${params}`);
        if (requestID !== request.current) {
          return response;
        }
        if (response.has_more && response.next_cursor) {
          cursors.current[page + 1] = response.next_cursor;
        } else {
          delete cursors.current[page + 1];
        }
        latestData.current = response;
        setData(response);
        return response;
      } finally {
        if (requestID === request.current) {
          setLoading(false);
        }
      }
    },
    [api, query],
  );

  return {
    data,
    filters,
    loading,
    setFilters,
    resetFilters: () => setFilters(EMPTY_FILTERS),
    load,
  };
}
