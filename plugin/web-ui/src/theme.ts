export type HostTheme = "light" | "white" | "dark";

const THEME_ATTRIBUTES = new Set(["light", "white", "dark"]);

export function normalizeHostTheme(
  value: string | null | undefined,
): HostTheme | null {
  return THEME_ATTRIBUTES.has(String(value)) ? (value as HostTheme) : null;
}

// The parent page decides the theme: both management centers keep the same
// data-theme vocabulary on their own documentElement. An absent attribute
// still counts as "light" (the centers drop the attribute for the default
// theme), while undefined means the parent told us nothing (standalone open
// or a cross-origin host) and the pre-set own attribute gets a chance.
export function resolveHostTheme(
  parentAttribute: string | null | undefined,
  ownAttribute: string | null | undefined,
): HostTheme {
  if (parentAttribute !== undefined) {
    return normalizeHostTheme(parentAttribute) ?? "light";
  }
  return normalizeHostTheme(ownAttribute) ?? "light";
}

export function applyHostTheme(theme: HostTheme): void {
  const root = document.documentElement;
  if (theme === "light") {
    if (root.hasAttribute("data-theme")) {
      root.removeAttribute("data-theme");
    }
    return;
  }
  if (root.getAttribute("data-theme") !== theme) {
    root.setAttribute("data-theme", theme);
  }
}

function parentThemeAttribute(): string | null | undefined {
  try {
    if (window.parent === window) {
      return undefined;
    }
    return window.parent.document.documentElement.getAttribute("data-theme");
  } catch {
    return undefined;
  }
}

// Host pages share origin with this page (same CPA server), so the theme is
// read straight from the parent document — the same trust boundary auth.ts
// already relies on for the management key. CPA-Manager-Plus additionally
// writes data-theme onto this document itself; the parent stays the source of
// truth so live host theme switches keep propagating.
export function initThemeSync(): () => void {
  const refresh = () => {
    applyHostTheme(
      resolveHostTheme(
        parentThemeAttribute(),
        document.documentElement.getAttribute("data-theme"),
      ),
    );
  };

  refresh();

  try {
    if (window.parent !== window) {
      const observer = new MutationObserver(refresh);
      observer.observe(window.parent.document.documentElement, {
        attributes: true,
        attributeFilter: ["data-theme"],
      });
      return () => observer.disconnect();
    }
  } catch {
    // Parent unreachable: keep the initially resolved theme.
  }
  return () => undefined;
}
