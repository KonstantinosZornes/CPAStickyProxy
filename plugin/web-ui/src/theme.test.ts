import { afterEach, describe, expect, it, vi } from "vitest";
import {
  applyHostTheme,
  initThemeSync,
  normalizeHostTheme,
  resolveHostTheme,
} from "./theme";

function fakeElement() {
  const attrs = new Map<string, string>();
  return {
    getAttribute: (name: string) => (attrs.has(name) ? attrs.get(name)! : null),
    setAttribute: (name: string, value: string) => attrs.set(name, value),
    removeAttribute: (name: string) => attrs.delete(name),
    hasAttribute: (name: string) => attrs.has(name),
  };
}

type FakeElement = ReturnType<typeof fakeElement>;

class FakeMutationObserver {
  static instances: FakeMutationObserver[] = [];

  callback: () => void;
  observed: { target: unknown; options: unknown } | null = null;
  disconnected = false;

  constructor(callback: () => void) {
    this.callback = callback;
    FakeMutationObserver.instances.push(this);
  }

  observe(target: unknown, options: unknown): void {
    this.observed = { target, options };
  }

  disconnect(): void {
    this.disconnected = true;
  }
}

function stubWindow(
  windowObject: Record<string, unknown> | object,
): FakeElement {
  const root = fakeElement();
  vi.stubGlobal("document", { documentElement: root });
  vi.stubGlobal("MutationObserver", FakeMutationObserver);
  vi.stubGlobal("window", windowObject);
  return root;
}

describe("host theme normalization", () => {
  it("accepts the management center theme vocabulary", () => {
    expect(normalizeHostTheme("dark")).toBe("dark");
    expect(normalizeHostTheme("white")).toBe("white");
    expect(normalizeHostTheme("light")).toBe("light");
  });

  it("rejects absent or unknown values", () => {
    expect(normalizeHostTheme(null)).toBeNull();
    expect(normalizeHostTheme(undefined)).toBeNull();
    expect(normalizeHostTheme("")).toBeNull();
    expect(normalizeHostTheme("auto")).toBeNull();
    expect(normalizeHostTheme("DARK")).toBeNull();
  });
});

describe("host theme resolution", () => {
  it("follows the parent attribute while the parent is readable", () => {
    expect(resolveHostTheme("dark", "white")).toBe("dark");
    expect(resolveHostTheme("white", "dark")).toBe("white");
  });

  it("treats a dropped parent attribute as the default light theme", () => {
    // Management centers remove data-theme for the default/auto theme.
    expect(resolveHostTheme(null, "dark")).toBe("light");
    expect(resolveHostTheme(null, undefined)).toBe("light");
  });

  it("falls back to a pre-set own attribute when the parent tells nothing", () => {
    expect(resolveHostTheme(undefined, "dark")).toBe("dark");
    expect(resolveHostTheme(undefined, "white")).toBe("white");
  });

  it("defaults to light without any theme information", () => {
    expect(resolveHostTheme(undefined, null)).toBe("light");
    expect(resolveHostTheme(undefined, undefined)).toBe("light");
    expect(resolveHostTheme(undefined, "junk")).toBe("light");
  });
});

describe("applyHostTheme", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("writes white/dark onto the document root and drops light", () => {
    const root = fakeElement();
    vi.stubGlobal("document", { documentElement: root });

    applyHostTheme("dark");
    expect(root.getAttribute("data-theme")).toBe("dark");

    applyHostTheme("white");
    expect(root.getAttribute("data-theme")).toBe("white");

    applyHostTheme("light");
    expect(root.hasAttribute("data-theme")).toBe(false);
  });
});

describe("initThemeSync", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    FakeMutationObserver.instances = [];
  });

  it("applies the parent theme and observes parent data-theme changes", () => {
    const parentRoot = fakeElement();
    parentRoot.setAttribute("data-theme", "dark");
    const root = stubWindow({
      parent: { document: { documentElement: parentRoot } },
    });

    const dispose = initThemeSync();
    expect(root.getAttribute("data-theme")).toBe("dark");
    expect(FakeMutationObserver.instances).toHaveLength(1);

    const observer = FakeMutationObserver.instances[0];
    expect(observer.observed).toEqual({
      target: parentRoot,
      options: { attributes: true, attributeFilter: ["data-theme"] },
    });

    dispose();
    expect(observer.disconnected).toBe(true);
  });

  it("follows live host switches without freezing on the last theme", () => {
    const parentRoot = fakeElement();
    const root = stubWindow({
      parent: { document: { documentElement: parentRoot } },
    });

    parentRoot.setAttribute("data-theme", "dark");
    initThemeSync();
    expect(root.getAttribute("data-theme")).toBe("dark");

    const observer = FakeMutationObserver.instances[0];

    parentRoot.setAttribute("data-theme", "white");
    observer.callback();
    expect(root.getAttribute("data-theme")).toBe("white");

    // Hosts drop the attribute for the default/auto theme; the page must not
    // stay stuck on the previously applied white/dark palette.
    parentRoot.removeAttribute("data-theme");
    observer.callback();
    expect(root.hasAttribute("data-theme")).toBe(false);
  });

  it("keeps a pre-set own attribute when the parent is unreadable", () => {
    const root = fakeElement();
    root.setAttribute("data-theme", "dark");
    stubWindow({
      // Reading a cross-origin parent document throws.
      get parent() {
        throw new Error("cross-origin");
      },
    });

    const dispose = initThemeSync();
    expect(root.getAttribute("data-theme")).toBe("dark");
    expect(FakeMutationObserver.instances).toHaveLength(0);
    dispose();
  });

  it("defaults to the light palette when opened standalone", () => {
    const root = fakeElement();
    const win: Record<string, unknown> = {};
    win.parent = win;
    stubWindow(win);

    const dispose = initThemeSync();
    expect(root.hasAttribute("data-theme")).toBe(false);
    expect(FakeMutationObserver.instances).toHaveLength(0);
    dispose();
  });
});
