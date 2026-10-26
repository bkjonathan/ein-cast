import type { LibraryItem, MediaKind } from "@/shared/bridge";

export type KindFilter = "all" | MediaKind;
export type SortKey = "name" | "newest" | "largest";

export const sortLabels: Record<SortKey, string> = {
  name: "Name",
  newest: "Newest",
  largest: "Largest",
};

const byName = (a: LibraryItem, b: LibraryItem) =>
  a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });

const comparators: Record<SortKey, (a: LibraryItem, b: LibraryItem) => number> = {
  name: byName,
  newest: (a, b) => b.modTime - a.modTime || byName(a, b),
  largest: (a, b) => b.size - a.size || byName(a, b),
};

/** Items per kind, for the chips. Counts ignore the search box on purpose. */
export function countByKind(items: readonly LibraryItem[]): Record<KindFilter, number> {
  const counts: Record<KindFilter, number> = { all: items.length, video: 0, audio: 0, image: 0 };
  for (const it of items) counts[it.kind]++;
  return counts;
}

/**
 * Applies the library and kind choices, the search box and the sort order. Search is a
 * case-insensitive substring match on the path inside the library (so a folder
 * name finds everything in it); every word must match somewhere, in any order.
 * The input array is never reordered.
 */
export function browseItems(
  items: readonly LibraryItem[],
  kind: KindFilter,
  query: string,
  sort: SortKey,
  library = "all",
): LibraryItem[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  const shown = items.filter((it) => {
    if (library !== "all" && it.libraryId !== library) return false;
    if (kind !== "all" && it.kind !== kind) return false;
    if (words.length === 0) return true;
    const haystack = it.relPath.toLowerCase();
    return words.every((w) => haystack.includes(w));
  });
  return shown.sort(comparators[sort]);
}
