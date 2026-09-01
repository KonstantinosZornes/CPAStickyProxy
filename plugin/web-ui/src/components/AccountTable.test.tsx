import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AccountTable } from "./AccountTable";
import type { AccountItem } from "../types";

describe("AccountTable", () => {
  it("renders the empty state when a malformed accounts response has null items", () => {
    const markup = renderToStaticMarkup(
      <AccountTable
        language="en"
        items={null as unknown as AccountItem[]}
        selection={{ kind: "explicit", authIndexes: new Set() }}
        onToggle={() => undefined}
      />,
    );

    expect(markup).toContain('class="empty"');
    expect(markup).toContain("No matching accounts");
  });
});
