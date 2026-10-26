import type { KindCounts, LibraryState } from "@/shared/bridge";

/** What all the libraries add up to. */
export interface Totals {
  total: number;
  totalSize: number;
  counts: KindCounts;
}

export function totalsOf(libraries: readonly LibraryState[]): Totals {
  const sum: Totals = { total: 0, totalSize: 0, counts: { video: 0, audio: 0, image: 0 } };
  for (const l of libraries) {
    sum.total += l.total;
    sum.totalSize += l.totalSize;
    sum.counts.video += l.counts.video;
    sum.counts.audio += l.counts.audio;
    sum.counts.image += l.counts.image;
  }
  return sum;
}

/**
 * The very first scan, with nothing to show yet: a library is being scanned and
 * none has any files or a problem. The app then shows a "Scanning…" screen
 * rather than an empty dashboard.
 */
export function isFirstScan(libraries: readonly LibraryState[]): boolean {
  return (
    libraries.some((l) => l.scanning) && libraries.every((l) => l.total === 0 && l.error === "")
  );
}

/** Library ID → name, for captions on files. */
export function nameById(libraries: readonly LibraryState[]): ReadonlyMap<string, string> {
  return new Map(libraries.map((l) => [l.id, l.name]));
}

/**
 * The filter to apply: the chosen library if it still exists, otherwise all. A
 * library removed while it was selected must not leave an empty list behind.
 */
export function effectiveLibrary(chosen: string, libraries: readonly LibraryState[]): string {
  return chosen !== "all" && libraries.some((l) => l.id === chosen) ? chosen : "all";
}
