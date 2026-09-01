import { t, type Language } from "../i18n";
import { proxyStateCategory } from "../proxyState";
import type { AccountItem, AccountSelection } from "../types";

type Props = {
  language: Language;
  items: AccountItem[];
  selection: AccountSelection;
  onToggle: (authIndex: string) => void;
};

function selected(selection: AccountSelection, authIndex: string): boolean {
  return selection.kind === "explicit"
    ? selection.authIndexes.has(authIndex)
    : !selection.excludedAuthIndexes.has(authIndex);
}

function proxyStateLabel(
  account: AccountItem,
  language: Language,
): string | undefined {
  const category = proxyStateCategory(account.proxy_state);
  if (!category) {
    return undefined;
  }
  const label = t(category, language);
  return category === "sticky_applied" && account.proxy_name
    ? `${label} · ${account.proxy_name}`
    : label;
}

export function AccountTable({ language, items, selection, onToggle }: Props) {
  const accounts = Array.isArray(items) ? items : [];
  if (!accounts.length) {
    return <div className="empty">{t("no_accounts", language)}</div>;
  }

  return (
    <div className="account-list">
      {accounts.map((account) => {
        const proxyLabel = proxyStateLabel(account, language);
        return (
          <article
            className={`account-row ${account.selectable ? "" : "account-disabled"}`}
            key={account.auth_index}
          >
            <input
              type="checkbox"
              checked={
                account.selectable && selected(selection, account.auth_index)
              }
              disabled={!account.selectable}
              onChange={() => onToggle(account.auth_index)}
              aria-label={account.email || account.name}
            />
            <div className="account-main">
              <div className="account-name">
                <strong>
                  {account.email || account.name || account.auth_index}
                </strong>
                {proxyLabel && (
                  <span className={`tag ${account.proxy_state}`}>
                    {proxyLabel}
                  </span>
                )}
              </div>
              <div className="account-meta">
                {account.name} · {account.provider || "—"} ·{" "}
                {account.type || account.account_type || "—"} ·{" "}
                {account.status || "—"} · priority {account.priority}
                {account.note ? ` · ${account.note}` : ""}
              </div>
              {account.masked_proxy_url && (
                <div className="account-url">{account.masked_proxy_url}</div>
              )}
            </div>
          </article>
        );
      })}
    </div>
  );
}
