import { afterEach, describe, expect, it, vi } from "vitest";
import { createApiClient } from "./apiClient";

describe("management API error translation", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses the backend code and ignores backend error text", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          ok: false,
          code: "invalid_proxy_name",
          error: "server text must never be shown",
        }),
        { status: 400, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const api = createApiClient(
      { apiBase: "https://cpa.example", managementKey: "test-key" },
      "en",
    );

    await expect(api("/proxies")).rejects.toMatchObject({
      code: "invalid_proxy_name",
      message:
        "Proxy name may contain only letters, numbers, hyphens, and underscores",
    });
  });

  it.each([
    ["en", "missing_proxy_password", "1024Proxy requires a proxy password"],
    ["zh", "invalid_1024_sid", "1024Proxy 用户名中的 session ID 格式无效"],
  ] as const)(
    "translates 1024Proxy error code %s in %s",
    async (language, code, message) => {
      vi.stubGlobal(
        "fetch",
        vi
          .fn()
          .mockResolvedValue(
            new Response(JSON.stringify({ ok: false, code }), { status: 400 }),
          ),
      );
      const api = createApiClient(
        { apiBase: "https://cpa.example", managementKey: "test-key" },
        language,
      );

      await expect(api("/proxies")).rejects.toMatchObject({ code, message });
    },
  );
});
