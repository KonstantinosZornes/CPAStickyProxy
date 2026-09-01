import { errorMessage, type Language } from "./i18n";
import type { ApiFailure, AuthContext } from "./types";

export class ManagementError extends Error {
  readonly code: string;
  readonly status: number;
  readonly payload: ApiFailure;

  constructor(
    code: string,
    status: number,
    payload: ApiFailure,
    language: Language,
  ) {
    super(errorMessage(code, language));
    this.name = "ManagementError";
    this.code = code;
    this.status = status;
    this.payload = payload;
  }
}

export type ApiClient = <T>(path: string, options?: RequestInit) => Promise<T>;

/**
 * Deep transport adapter: management responses either become typed data or a
 * translated error code. The browser never uses a backend-provided error text.
 */
export function createApiClient(
  auth: AuthContext | null,
  language: Language,
): ApiClient {
  return async function request<T>(
    path: string,
    options: RequestInit = {},
  ): Promise<T> {
    if (!auth) {
      throw new ManagementError(
        "auth_required",
        401,
        { ok: false, code: "auth_required" },
        language,
      );
    }

    const authorization = /^bearer\s+/i.test(auth.managementKey)
      ? auth.managementKey
      : `Bearer ${auth.managementKey}`;
    const response = await fetch(
      `${auth.apiBase}/v0/management/stickyproxy${path}`,
      {
        ...options,
        headers: {
          "Content-Type": "application/json",
          "Accept-Language": language === "zh" ? "zh-CN" : "en",
          Authorization: authorization,
          ...(options.headers || {}),
        },
      },
    );
    const payload = (await response.json().catch(() => ({}))) as ApiFailure;

    if (!response.ok || payload.ok === false) {
      const code =
        typeof payload.code === "string" ? payload.code : "request_failed";
      throw new ManagementError(code, response.status, payload, language);
    }

    return payload as T;
  };
}
