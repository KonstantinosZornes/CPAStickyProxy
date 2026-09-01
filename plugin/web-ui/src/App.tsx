import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createApiClient, type ApiClient } from "./apiClient";
import { readAuth, readLanguage } from "./auth";
import { t, type Language } from "./i18n";
import { useAccounts, useProxyState } from "./hooks";
import {
  emptySelection,
  invertSelection,
  selectAllFiltered as makeFilteredSelection,
  selectionPayload,
  toggleSelection,
} from "./selection";
import type {
  AccountFilters,
  AccountSelection,
  ProxyItem,
  SyncResult,
  TestResult,
  Toast,
} from "./types";
import { AccountSelectionDialog } from "./components/AccountSelectionDialog";
import { BulkProxyForm } from "./components/BulkProxyForm";
import { ProxyForm } from "./components/ProxyForm";
import { ProxyList } from "./components/ProxyList";
import { clearToastDismiss, scheduleToastDismiss } from "./toastTimer";
import { mutateThenRefresh } from "./mutationRefresh";
import "./styles.css";

type Dialog =
  | { type: "proxy-form"; proxy?: ProxyItem }
  | { type: "bulk" }
  | {
      type: "account-selection";
      mode: "apply" | "clear" | "clear-applied";
      proxy?: ProxyItem;
    }
  | null;

function operationMessage(
  result: SyncResult | undefined,
  language: Language,
): string {
  if (!result) {
    return "";
  }
  return t("apply_summary", language, {
    selected: result.selected,
    eligible: result.eligible,
    updated: result.updated,
    current: result.already_applied,
    skipped: result.skipped,
  });
}

export function App() {
  const language = readLanguage();
  const [auth] = useState(readAuth);
  const api = useMemo(() => createApiClient(auth, language), [auth, language]);
  const {
    state,
    loading: proxiesLoading,
    reload: reloadProxies,
  } = useProxyState(api);
  const accounts = useAccounts(api);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [selection, setSelection] = useState<AccountSelection>(emptySelection);
  const [tests, setTests] = useState<Record<string, TestResult | undefined>>(
    {},
  );
  const [previewProxyID, setPreviewProxyID] = useState("");
  const [previewEmail, setPreviewEmail] = useState("");
  const [preview, setPreview] = useState("");
  const [toast, setToast] = useState<Toast | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );

  useEffect(() => () => clearToastDismiss(toastTimer), []);

  const notify = useCallback((message: string, error = false) => {
    setToast({ message, error });
    scheduleToastDismiss(toastTimer, () => setToast(null));
  }, []);

  const loadAccountsPage = useCallback(
    async (page: number) => {
      try {
        await accounts.load(page);
      } catch (error) {
        notify(
          error instanceof Error
            ? error.message
            : t("request_failed", language),
          true,
        );
      }
    },
    [accounts.load, language, notify],
  );

  const action = useCallback(
    async <T,>(path: string, options?: RequestInit): Promise<T> => {
      try {
        return await mutateThenRefresh(
          () => api<T>(path, options),
          reloadProxies,
        );
      } catch (error) {
        notify(
          error instanceof Error
            ? error.message
            : t("request_failed", language),
          true,
        );
        throw error;
      }
    },
    [api, language, notify, reloadProxies],
  );

  const clearSelection = () => {
    setSelection(emptySelection());
  };

  const changeAccountFilters = (filters: AccountFilters) => {
    clearSelection();
    accounts.setFilters(filters);
  };

  const toggleAccount = (authIndex: string) => {
    setSelection((previous) => toggleSelection(previous, authIndex));
  };

  const selectAllFiltered = () => {
    setSelection(makeFilteredSelection(accounts.filters));
  };

  const invertCurrentFilterSelection = () => {
    setSelection((previous) => invertSelection(previous, accounts.filters));
  };

  const closeAccountSelection = () => {
    if (!submitting) {
      clearSelection();
      setDialog(null);
    }
  };

  const openAccountSelection = (
    mode: "apply" | "clear" | "clear-applied",
    proxy?: ProxyItem,
  ) => {
    if ((mode === "apply" || mode === "clear-applied") && !proxy) {
      return;
    }
    clearSelection();
    setDialog({ type: "account-selection", mode, proxy });
  };

  const applySelected = async (proxy: ProxyItem) => {
    if (submitting) {
      return;
    }
    setSubmitting(true);
    try {
      const response = await action<{ sync: SyncResult }>("/apply", {
        method: "POST",
        body: JSON.stringify({
          proxy_id: proxy.id,
          ...selectionPayload(selection),
        }),
      });
      notify(
        `${t("applied", language)} · ${operationMessage(response.sync, language)}`,
      );
      clearSelection();
      setDialog(null);
    } finally {
      setSubmitting(false);
    }
  };

  const clearSelected = async (proxy?: ProxyItem) => {
    if (submitting) {
      return;
    }
    setSubmitting(true);
    try {
      const isProxyScoped = Boolean(proxy);
      const response = await action<{ sync: SyncResult }>(
        isProxyScoped ? "/clear-applied" : "/clear-proxy",
        {
          method: "POST",
          body: JSON.stringify({
            ...(proxy ? { proxy_id: proxy.id } : {}),
            ...selectionPayload(selection),
          }),
        },
      );
      notify(
        `${t(isProxyScoped ? "cleared_from_proxy" : "cleared", language)} · ${operationMessage(response.sync, language)}`,
      );
      clearSelection();
      setDialog(null);
    } finally {
      setSubmitting(false);
    }
  };

  const confirmAccountSelection = async () => {
    if (!dialog || dialog.type !== "account-selection") {
      return;
    }
    try {
      if (dialog.mode === "apply" && dialog.proxy) {
        await applySelected(dialog.proxy);
        return;
      }
      await clearSelected(
        dialog.mode === "clear-applied" ? dialog.proxy : undefined,
      );
    } catch {
      // action already surfaced the failure as an error toast.
    }
  };

  const deleteProxy = async (proxy: ProxyItem) => {
    if (!window.confirm(t("confirm_delete_proxy", language))) {
      return;
    }
    try {
      await action("/proxies/delete", {
        method: "POST",
        body: JSON.stringify({ proxy_id: proxy.id }),
      });
    } catch {
      // action already surfaced the failure as an error toast.
      return;
    }
    notify(t("deleted", language));
    if (previewProxyID === proxy.id) {
      setPreviewProxyID("");
      setPreview("");
    }
  };

  const testProxy = async (proxy: ProxyItem) => {
    try {
      const response = await api<{ test: TestResult }>("/proxies/test", {
        method: "POST",
        body: JSON.stringify({ proxy_id: proxy.id }),
      });
      setTests((previous) => ({ ...previous, [proxy.id]: response.test }));
    } catch (error) {
      notify(
        error instanceof Error ? error.message : t("request_failed", language),
        true,
      );
    }
  };

  const previewProxy = async () => {
    if (!previewProxyID) {
      notify(t("proxy_required", language), true);
      return;
    }
    try {
      const response = await api<{
        masked_effective_url: string;
        notice_code?: string;
      }>("/preview", {
        method: "POST",
        body: JSON.stringify({
          proxy_id: previewProxyID,
          email: previewEmail,
        }),
      });
      setPreview(
        `${response.masked_effective_url}${
          response.notice_code ? `\n${t(response.notice_code, language)}` : ""
        }`,
      );
    } catch (error) {
      setPreview(
        error instanceof Error ? error.message : t("request_failed", language),
      );
    }
  };

  if (!auth) {
    return (
      <main className="shell">
        <section className="auth-required">
          <b>{t("auth_required", language)}</b>
        </section>
      </main>
    );
  }

  return (
    <main className="shell">
      <section className="section">
        <header className="section-head">
          <div>
            <h2>{t("proxies", language)}</h2>
            <p>{t("apply_proxy_hint", language)}</p>
          </div>
          <div className="section-actions">
            <button
              className="btn"
              onClick={() => openAccountSelection("clear")}
            >
              {t("proxy_view", language)}
            </button>
            <button className="btn" onClick={() => setDialog({ type: "bulk" })}>
              {t("bulk_add", language)}
            </button>
            <button
              className="btn primary"
              onClick={() => setDialog({ type: "proxy-form" })}
            >
              {t("add_proxy", language)}
            </button>
          </div>
        </header>
        {proxiesLoading ? (
          <div className="empty">…</div>
        ) : (
          <ProxyList
            items={state.proxies}
            language={language}
            tests={tests}
            onApply={(proxy) => openAccountSelection("apply", proxy)}
            onClearApplied={(proxy) =>
              openAccountSelection("clear-applied", proxy)
            }
            onEdit={(proxy) => setDialog({ type: "proxy-form", proxy })}
            onDelete={(proxy) => void deleteProxy(proxy)}
            onTest={(proxy) => void testProxy(proxy)}
          />
        )}
      </section>

      <section className="section preview">
        <header className="section-head">
          <div>
            <h2>{t("preview", language)}</h2>
          </div>
        </header>
        <div className="preview-row">
          <label className="field">
            <span>{t("proxy", language)}</span>
            <select
              className="select"
              value={previewProxyID}
              onChange={(event) => setPreviewProxyID(event.target.value)}
            >
              <option value="">{t("select_proxy", language)}</option>
              {state.proxies.map((proxy) => (
                <option key={proxy.id} value={proxy.id}>
                  {proxy.name}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>{t("email", language)}</span>
            <input
              className="input"
              type="email"
              value={previewEmail}
              onChange={(event) => setPreviewEmail(event.target.value)}
            />
          </label>
          <button className="btn primary" onClick={() => void previewProxy()}>
            {t("preview", language)}
          </button>
        </div>
        <pre className="preview-result">
          {preview || t("preview_placeholder", language)}
        </pre>
      </section>

      {dialog?.type === "proxy-form" && (
        <ProxyForm
          language={language}
          proxy={dialog.proxy}
          onClose={() => setDialog(null)}
          onSave={async (input) => {
            await action("/proxies", {
              method: "POST",
              body: JSON.stringify(input),
            });
            notify(t("saved", language));
          }}
        />
      )}

      {dialog?.type === "bulk" && (
        <BulkProxyForm
          language={language}
          onClose={() => setDialog(null)}
          onSave={async (input) => {
            const response = await action<{
              created: number;
              rejected: number;
            }>("/proxies/bulk", {
              method: "POST",
              body: JSON.stringify(input),
            });
            if (response.rejected > 0) {
              notify(
                t("bulk_partially_saved", language, {
                  created: response.created,
                  rejected: response.rejected,
                }),
                true,
              );
              return;
            }
            notify(t("saved", language));
          }}
        />
      )}

      {dialog?.type === "account-selection" && (
        <AccountSelectionDialog
          mode={dialog.mode}
          language={language}
          proxy={dialog.proxy}
          proxies={state.proxies}
          page={accounts.data}
          filters={accounts.filters}
          selection={selection}
          loading={accounts.loading}
          submitting={submitting}
          onFiltersChange={changeAccountFilters}
          onLoadPage={loadAccountsPage}
          onToggle={toggleAccount}
          onSelectAll={selectAllFiltered}
          onInvert={invertCurrentFilterSelection}
          onSelectNone={clearSelection}
          onConfirm={() => void confirmAccountSelection()}
          onClose={closeAccountSelection}
        />
      )}

      {toast && (
        <div className={`toast show ${toast.error ? "error" : ""}`}>
          {toast.message}
        </div>
      )}
    </main>
  );
}
