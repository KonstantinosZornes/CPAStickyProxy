import { useState } from "react";
import { t, type Language } from "../i18n";
import type { ProxyPlatform } from "../types";
import { Modal } from "./Modal";

type Props = {
  language: Language;
  onSave: (input: { platform: ProxyPlatform; text: string }) => Promise<void>;
  onClose: () => void;
};

export function BulkProxyForm({ language, onSave, onClose }: Props) {
  const [platform, setPlatform] = useState<ProxyPlatform>("decodo");
  const [text, setText] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!text.trim()) {
      setError(t("proxy_empty", language));
      return;
    }
    setSaving(true);
    setError("");
    try {
      await onSave({ platform, text });
      onClose();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : t("request_failed", language),
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal title={t("bulk_add", language)} onClose={onClose}>
      <form className="form-grid" onSubmit={submit}>
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
          <textarea
            className="textarea"
            value={text}
            onChange={(event) => setText(event.target.value)}
            placeholder="socks5://username:password@proxy.example:1080|platform"
          />
          <small>URL|platform</small>
        </label>
        {error && <p className="form-error">{error}</p>}
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
