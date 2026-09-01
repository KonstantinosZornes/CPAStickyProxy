import type { AuthContext, Language } from "./types";

const AUTH_KEY = "cli-proxy-auth";
const LANGUAGE_KEY = "cli-proxy-language";
const OBFUSCATION_PREFIX = "enc::v1::";
const OBFUSCATION_SALT = "cli-proxy-api-webui::secure-storage";

type PersistedEnvelope = {
  state?: Record<string, unknown>;
};

function decodeAuthStorage(raw: string | null): unknown {
  if (!raw) {
    return null;
  }

  try {
    if (!raw.startsWith(OBFUSCATION_PREFIX)) {
      return JSON.parse(raw);
    }

    const bytes = Uint8Array.from(
      atob(raw.slice(OBFUSCATION_PREFIX.length)),
      (char) => char.charCodeAt(0),
    );
    const key = new TextEncoder().encode(
      `${OBFUSCATION_SALT}|${location.host}|${navigator.userAgent}`,
    );
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] ^= key[index % key.length];
    }
    return JSON.parse(new TextDecoder().decode(bytes));
  } catch {
    return null;
  }
}

function persistedState(raw: string | null): Record<string, unknown> | null {
  try {
    const decoded = JSON.parse(raw || "null") as PersistedEnvelope | null;
    if (!decoded || typeof decoded !== "object") {
      return null;
    }
    return decoded.state && typeof decoded.state === "object"
      ? decoded.state
      : (decoded as Record<string, unknown>);
  } catch {
    return null;
  }
}

export function readAuth(): AuthContext | null {
  const decoded = decodeAuthStorage(localStorage.getItem(AUTH_KEY));
  const envelope =
    decoded && typeof decoded === "object"
      ? (decoded as PersistedEnvelope & Record<string, unknown>)
      : null;
  const data =
    envelope?.state && typeof envelope.state === "object"
      ? envelope.state
      : envelope;
  const managementKey = data?.managementKey;

  if (typeof managementKey !== "string" || !managementKey.trim()) {
    return null;
  }

  const apiBase = String(data?.apiBase || location.origin)
    .replace(/\/?v0\/management\/?$/i, "")
    .replace(/\/+$/, "");

  return {
    apiBase: apiBase || location.origin,
    managementKey,
  };
}

export function normalizeLanguage(value: unknown): Language {
  return String(value || "")
    .toLowerCase()
    .startsWith("zh")
    ? "zh"
    : "en";
}

export function readLanguage(): Language {
  return normalizeLanguage(
    persistedState(localStorage.getItem(LANGUAGE_KEY))?.language ||
      navigator.language,
  );
}
