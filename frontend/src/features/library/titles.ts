import type { LibraryItem } from "@/shared/bridge";

/** "Heat (1995)" for a video with a known year, else the title (the file name when nothing better is known). */
export function itemTitle(item: LibraryItem): string {
  return item.kind === "video" && item.year > 0 ? `${item.title} (${item.year})` : item.title;
}

/** "S02E05" for an episode, "E05" without a season, "" for anything else. */
export function episodeCode(item: LibraryItem): string {
  if (item.show === "" || item.episode <= 0) return "";
  const e = String(item.episode).padStart(2, "0");
  return item.season > 0 ? `S${String(item.season).padStart(2, "0")}E${e}` : `E${e}`;
}
