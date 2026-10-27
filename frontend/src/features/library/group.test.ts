import { describe, expect, it } from "vitest";
import type { LibraryItem } from "@/shared/bridge";
import { listRows, showKey } from "./group";

const item = (name: string, over: Partial<LibraryItem> = {}): LibraryItem => ({
  id: name,
  libraryId: "L1",
  name,
  relPath: name,
  title: name,
  year: 0,
  show: "",
  season: 0,
  episode: 0,
  poster: false,
  kind: "video",
  size: 1,
  modTime: 1,
  ...over,
});
const ep = (show: string, season: number, episode: number, over: Partial<LibraryItem> = {}) =>
  item(`${show} S${season}E${episode}.mkv`, { show, season, episode, ...over });

const shape = (rows: ReturnType<typeof listRows>) =>
  rows.map((r) => (r.type === "item" || r.type === "episode" ? `${r.type}:${r.item.name}` : r.type === "show" ? `show:${r.name}` : `season:${r.season > 0 ? `Season ${r.season}` : "Episodes"}`));

describe("listRows", () => {
  it("keeps files that are not episodes as lines of their own", () => {
    const rows = listRows([item("a.mkv"), item("b.mp3", { kind: "audio" })], new Set(), false);
    expect(shape(rows)).toEqual(["item:a.mkv", "item:b.mp3"]);
  });

  it("gathers a series under one closed header, where its first episode would be", () => {
    const sorted = [item("a.mkv"), ep("Fargo", 1, 2), item("b.mkv"), ep("Fargo", 1, 1)];
    const rows = listRows(sorted, new Set(), false);
    expect(shape(rows)).toEqual(["item:a.mkv", "show:Fargo", "item:b.mkv"]);
    expect(rows[1]).toMatchObject({ type: "show", episodes: 2, seasons: 1, open: false });
  });

  it("opens a series to its seasons and episodes in order, whatever order they were sorted in", () => {
    const sorted = [ep("Fargo", 2, 1), ep("Fargo", 1, 10), ep("Fargo", 1, 2)];
    const rows = listRows(sorted, new Set([showKey("Fargo")]), false);
    expect(shape(rows)).toEqual([
      "show:Fargo",
      "season:Season 1", "episode:Fargo S1E2.mkv", "episode:Fargo S1E10.mkv",
      "season:Season 2", "episode:Fargo S2E1.mkv",
    ]);
    expect(rows[0]).toMatchObject({ seasons: 2, open: true });
  });

  it("calls episodes with no season just Episodes", () => {
    const rows = listRows([ep("Anime", 0, 2), ep("Anime", 0, 1)], new Set([showKey("Anime")]), false);
    expect(shape(rows)).toEqual(["show:Anime", "season:Episodes", "episode:Anime S0E1.mkv", "episode:Anime S0E2.mkv"]);
  });

  it("opens everything for a search, so matches are not hidden", () => {
    const rows = listRows([ep("Fargo", 1, 1), ep("Lost", 1, 1)], new Set(), true);
    expect(rows.filter((r) => r.type === "episode")).toHaveLength(2);
    expect(rows.filter((r) => r.type === "show").every((r) => r.type === "show" && r.open)).toBe(true);
  });

  it("treats spellings of a series name that differ in case as one series, and across libraries", () => {
    const rows = listRows([ep("Fargo", 1, 1, { libraryId: "A" }), ep("FARGO ", 1, 2, { libraryId: "B" })], new Set(), false);
    expect(shape(rows)).toEqual(["show:Fargo"]);
    expect(rows[0]).toMatchObject({ episodes: 2 });
  });

  it("uses the first episode as the series' picture", () => {
    const first = ep("Fargo", 1, 2);
    const rows = listRows([first, ep("Fargo", 1, 1)], new Set(), false);
    expect(rows[0]).toMatchObject({ type: "show", cover: first });
  });

  it("does not file an item with a show but no episode number as an episode", () => {
    expect(shape(listRows([item("x.mkv", { show: "Fargo" })], new Set(), false))).toEqual(["item:x.mkv"]);
  });

  it("does not reorder or change its input", () => {
    const sorted = [ep("Fargo", 1, 2), ep("Fargo", 1, 1)];
    const copy = [...sorted];
    listRows(sorted, new Set([showKey("Fargo")]), false);
    expect(sorted).toEqual(copy);
  });
});

describe("showKey", () => {
  it("ignores case, outer spaces and Unicode form", () => {
    expect(showKey("  Amélie ")).toBe(showKey("AMÉLIE"));
  });
});
