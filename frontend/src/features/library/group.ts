import type { LibraryItem } from "@/shared/bridge";

/** One line of the Library list. Shows and seasons are lines of their own so the list can stay flat and virtual. */
export type ListRow =
  | { type: "item"; item: LibraryItem }
  | { type: "show"; key: string; name: string; episodes: number; seasons: number; cover: LibraryItem; open: boolean }
  | { type: "season"; key: string; label: string }
  | { type: "episode"; item: LibraryItem };

/** Two spellings of a series name that differ only in case or Unicode form are one series. */
export function showKey(name: string): string {
  return name.normalize("NFC").trim().toLowerCase();
}

const byEpisode = (a: LibraryItem, b: LibraryItem) =>
  a.season - b.season ||
  a.episode - b.episode ||
  a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });

/**
 * Turns the sorted, filtered items into the lines to draw. Episodes of one series
 * are gathered under a header placed where its first episode would have been, so
 * the chosen sort still orders the list. A header's seasons and episodes show only
 * while it is open, or for every series when `openAll` is set (a search, whose
 * matches would otherwise hide behind closed headers). Everything that is not an
 * episode of a series stays a line of its own.
 */
export function listRows(
  sorted: readonly LibraryItem[],
  opened: ReadonlySet<string>,
  openAll: boolean,
): ListRow[] {
  const series = new Map<string, LibraryItem[]>();
  for (const it of sorted) {
    if (it.show === "" || it.episode <= 0) continue;
    const key = showKey(it.show);
    const list = series.get(key);
    if (list) list.push(it);
    else series.set(key, [it]);
  }

  const rows: ListRow[] = [];
  const emitted = new Set<string>();
  for (const it of sorted) {
    if (it.show === "" || it.episode <= 0) {
      rows.push({ type: "item", item: it });
      continue;
    }
    const key = showKey(it.show);
    if (emitted.has(key)) continue;
    emitted.add(key);

    const episodes = (series.get(key) ?? []).toSorted(byEpisode);
    const seasons = new Set(episodes.map((e) => e.season));
    const open = openAll || opened.has(key);
    rows.push({ type: "show", key, name: it.show, episodes: episodes.length, seasons: seasons.size, cover: it, open });
    if (!open) continue;

    let season = -1;
    for (const ep of episodes) {
      if (ep.season !== season) {
        season = ep.season;
        rows.push({ type: "season", key: `${key}|${season}`, label: season > 0 ? `Season ${season}` : "Episodes" });
      }
      rows.push({ type: "episode", item: ep });
    }
  }
  return rows;
}
