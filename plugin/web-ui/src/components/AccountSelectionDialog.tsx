import { useEffect, useRef } from "react";
import { t, type Language } from "../i18n";
import type {
  AccountFilters,
  AccountPage,
  AccountSelection,
  ProxyItem,
} from "../types";
import { AccountFiltersPanel } from "./AccountFilters";
import { AccountTable } from "./AccountTable";
import { Modal } from "./Modal";

type Mode = "apply" | "clear" | "clear-applied";

type Props = {
  mode: Mode;
  language: Language;
  proxy?: ProxyItem;
  proxies: ProxyItem[];
  page: AccountPage;
  filters: AccountFilters;
  selection: AccountSelection;
  loading: boolean;
  submitting: boolean;
  onFiltersChange: (filters: AccountFilters) => void;
  onLoadPage: (page: number) => void;
  onToggle: (authIndex: string) => void;
  onSelectAll: () => void;
  onInvert: () => void;
  onSelectNone: () => void;
  onConfirm: () => void;
  onClose: () => void;
};

export function AccountSelectionDialog({
  mode,
  language,
  proxy,
  proxies,
  page,
  filters,
  selection,
  loading,
  submitting,
  onFiltersChange,
  onLoadPage,
  onToggle,
  onSelectAll,
  onInvert,
  onSelectNone,
  onConfirm,
  onClose,
}: Props) {
  const results = useRef<HTMLDivElement>(null);
  const titleKey =
    mode === "clear"
      ? "dialog_clear_title"
      : mode === "clear-applied"
        ? "dialog_clear_applied_title"
        : "dialog_apply_title";
  const hintKey =
    mode === "clear"
      ? "dialog_clear_hint"
      : mode === "clear-applied"
        ? "dialog_clear_applied_hint"
        : "dialog_apply_hint";

  useEffect(() => {
    const timeout = window.setTimeout(() => onLoadPage(1), 180);
    return () => window.clearTimeout(timeout);
  }, [filters, onLoadPage]);

  useEffect(() => {
    results.current?.scrollTo({ top: 0 });
  }, [page.page, filters]);

  const actionKey = mode === "apply" ? "apply_button" : "clear_button";
  const explicitCount =
    selection.kind === "explicit" ? selection.authIndexes.size : 0;
  const hasSelection =
    selection.kind === "filtered" || selection.authIndexes.size > 0;
  const actionLabel =
    selection.kind === "filtered"
      ? t(
          mode === "apply"
            ? "apply_filtered_button"
            : mode === "clear-applied"
              ? "clear_applied_filtered_button"
              : "clear_filtered_button",
          language,
        )
      : t(actionKey, language, { count: explicitCount });

  return (
    <Modal
      title={t(titleKey, language)}
      onClose={onClose}
      dialogClassName="account-selection-dialog"
      busy={submitting}
      footer={
        <>
          <span className="selection-count">
            {selection.kind === "filtered"
              ? t("selected_filtered", language)
              : t("selected_count", language, {
                  selected: explicitCount,
                  total: page.total ?? "?",
                })}
          </span>
          <div className="dialog-actions">
            <button className="btn" disabled={submitting} onClick={onClose}>
              {t("cancel", language)}
            </button>
            <button
              className="btn primary"
              disabled={!hasSelection || loading || submitting}
              onClick={onConfirm}
            >
              {actionLabel}
            </button>
          </div>
        </>
      }
    >
      <div className="selection-dialog-fixed">
        <p className="dialog-hint">
          {t(hintKey, language, { proxy: proxy?.name || "—" })}
        </p>
        <AccountFiltersPanel
          language={language}
          filters={filters}
          facets={page.facets}
          proxies={proxies}
          onChange={onFiltersChange}
        />
      </div>
      <div ref={results} className="selection-dialog-results">
        <AccountTable
          language={language}
          items={page.items}
          selection={selection}
          onToggle={onToggle}
        />
      </div>
      <div className="pager selection-dialog-pager">
        <div className="toolbar-left">
          <button
            className="btn small"
            disabled={
              loading || submitting || (page.total_known && page.total === 0)
            }
            onClick={onSelectAll}
          >
            {t("select_all_filtered", language)}
          </button>
          <button
            className="btn small"
            disabled={submitting}
            onClick={onInvert}
          >
            {t("invert_selection", language)}
          </button>
          <button
            className="btn small"
            disabled={submitting}
            onClick={onSelectNone}
          >
            {t("select_none", language)}
          </button>
        </div>
        <div className="toolbar-right selection-dialog-pagination">
          <span>
            {page.total_known
              ? t("page_info", language, {
                  page: page.page,
                  total: page.total ?? 0,
                })
              : t("page_info_unknown_total", language, {
                  page: page.page,
                })}
          </span>
          <div className="pager-actions">
            <button
              className="btn small"
              disabled={loading || page.page <= 1}
              onClick={() => onLoadPage(page.page - 1)}
            >
              {t("previous", language)}
            </button>
            <button
              className="btn small"
              disabled={loading || !page.has_more}
              onClick={() => onLoadPage(page.page + 1)}
            >
              {t("next", language)}
            </button>
          </div>
        </div>
      </div>
    </Modal>
  );
}
