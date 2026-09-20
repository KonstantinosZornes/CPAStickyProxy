import { afterEach, describe, expect, it, vi } from "vitest";
import { normalizeLanguage, readAuth } from "./auth";

const OBFUSCATION_SALT = "cli-proxy-api-webui::secure-storage";

function obfuscate(plain: string, keyMaterial: string): string {
  const bytes = new TextEncoder().encode(plain);
  const key = new TextEncoder().encode(keyMaterial);
  for (let index = 0; index < bytes.length; index += 1) {
    bytes[index] ^= key[index % key.length];
  }
  let binary = "";
  for (let index = 0; index < bytes.length; index += 1) {
    binary += String.fromCharCode(bytes[index]);
  }
  return btoa(binary);
}

function stubHostPage(): void {
  vi.stubGlobal("location", { host: "cpa.local:8317", origin: "http://cpa.local:8317" });
  vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0 Test Browser" });
}

function stubAuthStorage(raw: string | null): void {
  vi.stubGlobal("localStorage", { getItem: () => raw });
}

describe("CPA language normalization", () => {
  it("uses Chinese only for zh language tags", () => {
    expect(normalizeLanguage("zh-CN")).toBe("zh");
    expect(normalizeLanguage("zh-TW")).toBe("zh");
  });

  it("defaults unsupported, empty, and English tags to English", () => {
    expect(normalizeLanguage("en-US")).toBe("en");
    expect(normalizeLanguage("ja-JP")).toBe("en");
    expect(normalizeLanguage("fr-FR")).toBe("en");
    expect(normalizeLanguage("")).toBe("en");
    expect(normalizeLanguage(undefined)).toBe("en");
  });
});

describe("readAuth across management center storage formats", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const envelope = JSON.stringify({
    state: { apiBase: "http://cpa.local:8317", managementKey: "test-key" },
    version: 0,
  });

  it("reads plaintext JSON", () => {
    stubHostPage();
    stubAuthStorage(envelope);

    expect(readAuth()).toEqual({
      apiBase: "http://cpa.local:8317",
      managementKey: "test-key",
    });
  });

  it("reads enc::v1:: ciphertext from the old management center", () => {
    stubHostPage();
    stubAuthStorage(`enc::v1::${obfuscate(envelope, `${OBFUSCATION_SALT}|cpa.local:8317|Mozilla/5.0 Test Browser`)}`);

    expect(readAuth()).toEqual({
      apiBase: "http://cpa.local:8317",
      managementKey: "test-key",
    });
  });

  it("reads enc::v2:: ciphertext from CPA-Manager-Plus", () => {
    stubHostPage();
    stubAuthStorage(`enc::v2::${obfuscate(envelope, `${OBFUSCATION_SALT}|v2|cpa.local:8317`)}`);

    expect(readAuth()).toEqual({
      apiBase: "http://cpa.local:8317",
      managementKey: "test-key",
    });
  });

  it("strips the management path suffix from apiBase", () => {
    stubHostPage();
    stubAuthStorage(
      JSON.stringify({
        state: { apiBase: "http://cpa.local:8317/v0/management/", managementKey: "k" },
      }),
    );

    expect(readAuth()).toEqual({ apiBase: "http://cpa.local:8317", managementKey: "k" });
  });

  it("returns null for undecodable payloads", () => {
    stubHostPage();
    stubAuthStorage("enc::v2::not-base64!!");

    expect(readAuth()).toBeNull();
  });

  it("returns null when the management key is missing", () => {
    stubHostPage();
    stubAuthStorage(JSON.stringify({ state: { apiBase: "http://cpa.local:8317" } }));

    expect(readAuth()).toBeNull();
  });
});
