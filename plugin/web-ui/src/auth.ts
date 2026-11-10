import type { AuthContext, Language } from "./types";

const AUTH_KEY = "cli-proxy-auth";
const LANGUAGE_KEY = "cli-proxy-language";
// Host pages share this storage entry with the embedded plugin page:
// the old management center and the CPA built-in web UI write enc::v1::,
// CPA-Manager-Plus writes enc::v2::. Both must stay readable.
const OBFUSCATION_PREFIX_V1 = "enc::v1::";
const OBFUSCATION_PREFIX_V2 = "enc::v2::";
const OBFUSCATION_SALT = "cli-proxy-api-webui::secure-storage";

type PersistedEnvelope = {
  state?: Record<string, unknown>;
};

type ObfuscationVersion = "v1" | "v2";

function obfuscationKeyCandidates(version: ObfuscationVersion): Uint8Array[] {
  try {
    const host = location.host;
    if (version === "v2") {
      // Current CPA-Manager-Plus uses the versioned key. Older deployed
      // builds wrote enc::v2:: with the host-only key, so keep both readable.
      return [
        new TextEncoder().encode(`${OBFUSCATION_SALT}|v2|${host}`),
        new TextEncoder().encode(`${OBFUSCATION_SALT}|${host}`),
      ];
    }
    return [new TextEncoder().encode(`${OBFUSCATION_SALT}|${host}|${navigator.userAgent}`)];
  } catch {
    return version === "v2"
      ? [
          new TextEncoder().encode(`${OBFUSCATION_SALT}|v2`),
          new TextEncoder().encode(OBFUSCATION_SALT),
        ]
      : [new TextEncoder().encode(OBFUSCATION_SALT)];
  }
}

function decodeObfuscated(raw: string, prefix: string, version: ObfuscationVersion): unknown {
  const encrypted = Uint8Array.from(atob(raw.slice(prefix.length)), (char) => char.charCodeAt(0));
  for (const key of obfuscationKeyCandidates(version)) {
    const bytes = encrypted.slice();
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] ^= key[index % key.length];
    }
    try {
      return JSON.parse(new TextDecoder().decode(bytes));
    } catch {
      // Try the next key derivation used by another CPA-Manager-Plus build.
    }
  }
  throw new Error("Unable to decode persisted authentication");
}

function decodeAuthStorage(raw: string | null): unknown {
  if (!raw) {
    return null;
  }

  try {
    if (raw.startsWith(OBFUSCATION_PREFIX_V2)) {
      return decodeObfuscated(raw, OBFUSCATION_PREFIX_V2, "v2");
    }
    if (raw.startsWith(OBFUSCATION_PREFIX_V1)) {
      return decodeObfuscated(raw, OBFUSCATION_PREFIX_V1, "v1");
    }
    return JSON.parse(raw);
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
