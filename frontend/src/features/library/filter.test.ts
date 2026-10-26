import { describe, expect, it } from "vitest";
import type { LibraryItem } from "@/shared/bridge";
import { browseItems, countByKind } from "./filter";

const item = (over: Partial<LibraryItem> & { relPath: string }): LibraryItem => ({
  id: over.relPath,
  name: over.relPath.split("/").at(-1) ?? over.relPath,
  kind: "video",
  size: 1,
  modTime: 1,
  ...over,
});

const items = [
  item({ relPath: "shows/Episode 10.mkv", size: 300, modTime: 10 }),
  item({ relPath: "shows/Episode 2.mkv", size: 100, modTime: 30 }),
  item({ relPath: "music/Song.flac", kind: "audio", size: 200, modTime: 20 }),
  item({ relPath: "photos/Beach.JPG", kind: "image", size: 50, modTime: 5 }),
];

const names = (list: LibraryItem[]) => list.map((i) => i.name);

describe("browseItems", () => {
  it("sorts by name with numbers in numeric order", () => {
    expect(names(browseItems(items, "all", "", "name"))).toEqual([
      "Beach.JPG",
      "Episode 2.mkv",
      "Episode 10.mkv",
      "Song.flac",
    ]);
  });

  it("sorts newest and largest first, breaking ties by name", () => {
    expect(names(browseItems(items, "all", "", "newest"))).toEqual([
      "Episode 2.mkv",
      "Song.flac",
      "Episode 10.mkv",
      "Beach.JPG",
    ]);
    expect(names(browseItems(items, "all", "", "largest"))[0]).toBe("Episode 10.mkv");
    const tied = [item({ relPath: "b.mp4" }), item({ relPath: "a.mp4" })];
    expect(names(browseItems(tied, "all", "", "largest"))).toEqual(["a.mp4", "b.mp4"]);
  });

  it("filters by kind", () => {
    expect(names(browseItems(items, "audio", "", "name"))).toEqual(["Song.flac"]);
  });

  it("matches case-insensitively, including folder names", () => {
    expect(names(browseItems(items, "all", "SHOWS", "name"))).toEqual([
      "Episode 2.mkv",
      "Episode 10.mkv",
    ]);
  });

  it("requires every word, in any order", () => {
    expect(names(browseItems(items, "all", "mkv  episode 10", "name"))).toEqual(["Episode 10.mkv"]);
    expect(browseItems(items, "all", "episode flac", "name")).toEqual([]);
  });

  it("combines the kind chip with the search", () => {
    expect(browseItems(items, "image", "song", "name")).toEqual([]);
  });

  it("treats a blank query as no search and never reorders its input", () => {
    const before = [...items];
    expect(browseItems(items, "all", "   ", "name")).toHaveLength(4);
    expect(items).toEqual(before);
  });
});

describe("countByKind", () => {
  it("counts every kind and the total", () => {
    expect(countByKind(items)).toEqual({ all: 4, video: 2, audio: 1, image: 1 });
    expect(countByKind([])).toEqual({ all: 0, video: 0, audio: 0, image: 0 });
  });
});
