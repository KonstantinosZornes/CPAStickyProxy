import { useState } from "react";
import { t, type Language } from "../i18n";
import type { ProxyItem, ProxyPlatform } from "../types";
import { Modal } from "./Modal";

type SaveInput = {
  id?: string;
  name: string;
  platform: ProxyPlatform;
  base_url: string;
};

type Props = {
  language: Language;
  proxy?: ProxyItem;
  onSave: (input: SaveInput) => Promise<void>;
  onClose: () => void;
};

const namePattern = /^[A-Za-z0-9_-]+$/;

export function ProxyForm({ language, proxy, onSave, onClose }: Props) {
  const [name, setName] = useState(proxy?.name || "");
  const [platform, setPlatform] = useState<ProxyPlatform>(
    proxy?.platform || "decodo",
  );
  const [baseURL, setBaseURL] = useState("");
  const [saving, setSaving] = useState(false);
  const [validationError, setValidationError] = useState("");

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!namePattern.test(name)) {
      setValidationError(t("invalid_proxy_name", language));
      return;
    }
    if (!proxy && !baseURL.trim()) {
      setValidationError(t("proxy_empty", language));
      return;
    }

    setSaving(true);
    setValidationError("");
    try {
      await onSave({
        id: proxy?.id,
        name,
        platform,
        base_url: baseURL,
      });
      onClose();
    } catch (error) {
      setValidationError(
        error instanceof Error ? error.message : t("request_failed", language),
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={t(proxy ? "edit_proxy" : "add_proxy", language)}
      onClose={onClose}
    >
      <form className="form-grid" onSubmit={submit}>
        <label className="field">
          <span>{t("proxy_name", language)}</span>
          <input
            className="input"
            value={name}
            maxLength={80}
            pattern="[A-Za-z0-9_-]+"
            onChange={(event) => setName(event.target.value)}
            required
          />
          <small>{t("proxy_name_help", language)}</small>
        </label>
        <label className="field">
          <span>{t("platform", language)}</span>
          <select
            className="select"
            value={platform}
            onChange={(event) =>
              setPlatform(event.target.value as ProxyPlatform)
            }
          >
            <option value="decodo">Decodo</option>
            <option value="dataimpulse">DataImpulse</option>
            <option value="resin">Resin</option>
            <option value="1024proxy">1024Proxy</option>
            <option value="generic">{t("generic_proxy", language)}</option>
          </select>
          {platform === "generic" && (
            <small>{t("generic_proxy_hint", language)}</small>
          )}
        </label>
        <label className="field">
          <span>{t("base_url", language)}</span>
          <input
            className="input"
            type="text"
            autoComplete="off"
            placeholder="http://username:password@host:port"
            value={baseURL}
            onChange={(event) => setBaseURL(event.target.value)}
            required={!proxy}
          />
          <small>
            {proxy
              ? t("keep_base_url", language)
              : t("base_url_help", language)}
          </small>
        </label>
        {validationError && <p className="form-error">{validationError}</p>}
        <div className="form-actions">
          <button className="btn" type="button" onClick={onClose}>
            {t("cancel", language)}
          </button>
          <button className="btn primary" disabled={saving}>
            {t("save", language)}
          </button>
        </div>
      </form>
    </Modal>
  );
}
