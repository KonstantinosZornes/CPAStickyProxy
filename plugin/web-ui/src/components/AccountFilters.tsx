import { useEffect, useState } from "react";
import {
  appliedProxyValue,
  updateAppliedProxyFilter,
  updateProxyStateFilter,
} from "../accountFilters";
import { t, type Language } from "../i18n";
import {
  PROXY_STATE_CATEGORIES,
  proxyStateFilterValue,
  proxyStateSelectValue,
} from "../proxyState";
import type { AccountFacets, AccountFilters, ProxyItem } from "../types";

type Props = {
  language: Language;
  filters: AccountFilters;
  facets: AccountFacets;
  proxies: ProxyItem[];
  onChange: (filters: AccountFilters) => void;
};

export function AccountFiltersPanel({
  language,
  filters,
  facets,
  proxies,
  onChange,
}: Props) {
  const [searchQuery, setSearchQuery] = useState(filters.q);

  useEffect(() => {
    setSearchQuery(filters.q);
  }, [filters.q]);

  const update = (key: keyof AccountFilters, value: string) => {
    onChange({ ...filters, [key]: value });
  };

  const submitSearch = () => {
    update("q", searchQuery);
  };

  return (
    <div className="filters">
      <label className="field filter-wide">
        <span>{t("search", language)}</span>
        <input
          className="input"
          value={searchQuery}
          onChange={(event) => setSearchQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              submitSearch();
            }
          }}
        />
      </label>
      <label className="field">
        <span>{t("provider", language)}</span>
        <select
          className="select"
          value={filters.provider}
          onChange={(event) => update("provider", event.target.value)}
        >
          <option value="">{t("all", language)}</option>
          {facets.providers.map((provider) => (
            <option key={provider} value={provider}>
              {provider}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>{t("account_type", language)}</span>
        <select
          className="select"
          value={filters.type}
          onChange={(event) => update("type", event.target.value)}
        >
          <option value="">{t("all", language)}</option>
          {facets.types.map((type) => (
            <option key={type} value={type}>
              {type}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>{t("account_status", language)}</span>
        <select
          className="select"
          value={filters.status}
          onChange={(event) => update("status", event.target.value)}
        >
          <option value="">{t("all", language)}</option>
          {facets.statuses.map((status) => (
            <option key={status} value={status}>
              {status}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>{t("proxy_state", language)}</span>
        <select
          className="select"
          value={proxyStateSelectValue(filters.proxy_state)}
          onChange={(event) =>
            onChange(updateProxyStateFilter(filters, event.target.value))
          }
        >
          <option value="">{t("all", language)}</option>
          {PROXY_STATE_CATEGORIES.map((category) => (
            <option key={category} value={proxyStateFilterValue(category)}>
              {t(category, language)}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>{t("applied_proxy", language)}</span>
        <select
          className="select"
          value={appliedProxyValue(filters)}
          onChange={(event) =>
            onChange(updateAppliedProxyFilter(filters, event.target.value))
          }
        >
          <option value="">{t("all", language)}</option>
          {proxies.map((proxy) => (
            <option key={proxy.id} value={proxy.id}>
              {proxy.name}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>{t("priority", language)}</span>
        <select
          className="select"
          value={filters.priority}
          onChange={(event) => update("priority", event.target.value)}
        >
          <option value="">{t("all", language)}</option>
          {facets.priorities.map((priority) => (
            <option key={priority} value={priority}>
              {priority}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>{t("note", language)}</span>
        <input
          className="input"
          value={filters.note}
          onChange={(event) => update("note", event.target.value)}
        />
      </label>
      <label className="field">
        <span>{t("enabled_state", language)}</span>
        <select
          className="select"
          value={filters.include_disabled}
          onChange={(event) => update("include_disabled", event.target.value)}
        >
          <option value="">{t("all", language)}</option>
          <option value="false">{t("enabled_only", language)}</option>
          <option value="true">{t("disabled_only", language)}</option>
        </select>
      </label>
    </div>
  );
}
