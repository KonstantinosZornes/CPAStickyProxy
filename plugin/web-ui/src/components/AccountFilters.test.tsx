import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AccountFiltersPanel } from "./AccountFilters";
import type { AccountFacets, AccountFilters } from "../types";

const filters: AccountFilters = {
  q: "",
  provider: "",
  type: "",
  status: "",
  proxy_state: "",
  proxy_id: "",
  include_disabled: "",
  priority: "",
  note: "",
};

const facets: AccountFacets = {
  providers: [],
  types: [],
  statuses: [],
  priorities: [],
};

describe("current applied proxy selector", () => {
  it("does not include the inherited system proxy category", () => {
    const markup = renderToStaticMarkup(
      <AccountFiltersPanel
        language="zh"
        filters={filters}
        facets={facets}
        proxies={[]}
        onChange={() => undefined}
      />,
    );
    const currentAppliedProxy = markup.slice(
      markup.indexOf("当前应用代理"),
      markup.indexOf("优先级"),
    );

    expect(currentAppliedProxy).not.toContain('value="inherits_system"');
    expect(currentAppliedProxy).not.toContain("继承系统代理");
  });
});
