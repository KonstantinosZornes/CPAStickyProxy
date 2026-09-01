import type { AccountFilters, AccountSelection } from "./types";

export function emptySelection(): AccountSelection {
  return { kind: "explicit", authIndexes: new Set() };
}

export function selectAllFiltered(filters: AccountFilters): AccountSelection {
  return {
    kind: "filtered",
    filters: { ...filters },
    excludedAuthIndexes: new Set(),
  };
}

// invertSelection keeps inversion compact: explicit selections become an
// all-filtered selection with those IDs excluded; all-filtered selections
// become the explicit set that was excluded.
export function invertSelection(
  selection: AccountSelection,
  filters: AccountFilters,
): AccountSelection {
  if (selection.kind === "filtered") {
    return {
      kind: "explicit",
      authIndexes: new Set(selection.excludedAuthIndexes),
    };
  }
  return {
    kind: "filtered",
    filters: { ...filters },
    excludedAuthIndexes: new Set(selection.authIndexes),
  };
}

export function toggleSelection(
  selection: AccountSelection,
  authIndex: string,
): AccountSelection {
  if (selection.kind === "filtered") {
    const excludedAuthIndexes = new Set(selection.excludedAuthIndexes);
    if (excludedAuthIndexes.has(authIndex)) {
      excludedAuthIndexes.delete(authIndex);
    } else {
      excludedAuthIndexes.add(authIndex);
    }
    return { ...selection, excludedAuthIndexes };
  }

  const authIndexes = new Set(selection.authIndexes);
  if (authIndexes.has(authIndex)) {
    authIndexes.delete(authIndex);
  } else {
    authIndexes.add(authIndex);
  }
  return { kind: "explicit", authIndexes };
}

export function selectionPayload(selection: AccountSelection): object {
  if (selection.kind === "explicit") {
    return { auth_indexes: [...selection.authIndexes] };
  }
  return {
    selection: {
      kind: "filtered",
      filters: selection.filters,
      excluded_auth_indexes: [...selection.excludedAuthIndexes],
    },
  };
}
