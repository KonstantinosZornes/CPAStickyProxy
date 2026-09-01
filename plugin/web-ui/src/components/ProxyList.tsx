import { t, type Language } from "../i18n";
import type { ProxyItem, ProxyPlatform, TestResult } from "../types";

type Props = {
  items: ProxyItem[];
  language: Language;
  tests: Record<string, TestResult | undefined>;
  onApply: (proxy: ProxyItem) => void;
  onClearApplied: (proxy: ProxyItem) => void;
  onEdit: (proxy: ProxyItem) => void;
  onDelete: (proxy: ProxyItem) => void;
  onTest: (proxy: ProxyItem) => void;
};

function platformMark(platform: ProxyPlatform): string {
  if (platform === "dataimpulse") return "DI";
  if (platform === "resin") return "R";
  if (platform === "1024proxy") return "1024";
  if (platform === "generic") return "G";
  return "D";
}

function platformName(platform: ProxyPlatform, language: Language): string {
  return platform === "generic" ? t("generic_proxy", language) : platform;
}

export function ProxyList({
  items,
  language,
  tests,
  onApply,
  onClearApplied,
  onEdit,
  onDelete,
  onTest,
}: Props) {
  if (!items.length) {
    return (
      <div className="empty">
        <b>{t("no_proxies", language)}</b>
      </div>
    );
  }

  return (
    <div className="proxy-list">
      {items.map((proxy) => {
        const result = tests[proxy.id];
        return (
          <article className="proxy-row" key={proxy.id}>
            <div className={`provider-mark ${proxy.platform}`}>
              {platformMark(proxy.platform)}
            </div>
            <div className="proxy-main">
              <div className="proxy-name">
                <strong>{proxy.name || proxy.platform}</strong>
                <span className="tag">{platformName(proxy.platform, language)}</span>
              </div>
              <div className="proxy-url">{proxy.masked_base_url}</div>
              {result && (
                <div className="test-result">
                  {t("test_result", language, {
                    ip: result.ip,
                    country: result.country || result.country_code || "—",
                    region: result.region || "—",
                    city: result.city || "—",
                  })}
                </div>
              )}
            </div>
            <div className="proxy-actions">
              <button
                className="btn small primary"
                onClick={() => onApply(proxy)}
              >
                {t("apply_proxy", language)}
              </button>
              <button
                className="btn small"
                onClick={() => onClearApplied(proxy)}
              >
                {t("clear_proxy_from_accounts", language)}
              </button>
              <button className="btn small" onClick={() => onTest(proxy)}>
                {t("test", language)}
              </button>
              <button
                className="icon-btn"
                onClick={() => onEdit(proxy)}
                aria-label={t("edit_proxy", language)}
              >
                ✎
              </button>
              <button
                className="icon-btn danger"
                onClick={() => onDelete(proxy)}
                aria-label={t("delete_proxy", language)}
              >
                ⌫
              </button>
            </div>
          </article>
        );
      })}
    </div>
  );
}
