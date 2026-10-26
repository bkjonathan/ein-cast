import { describe, expect, it } from "vitest";
import type { LibraryItem } from "@/shared/bridge";
import { episodeCode, itemTitle } from "./titles";

const item = (over: Partial<LibraryItem>): LibraryItem => ({
  id: "x", libraryId: "L", name: "f.mkv", relPath: "f.mkv", title: "f.mkv", year: 0, show: "", season: 0, episode: 0,
  poster: false, kind: "video", size: 1, modTime: 1, ...over,
});

describe("itemTitle", () => {
  it("adds the year to a video's title", () => {
    expect(itemTitle(item({ title: "Heat", year: 1995 }))).toBe("Heat (1995)");
  });
  it("shows the title alone when the year is unknown, and never invents one for a song or photo", () => {
    expect(itemTitle(item({ title: "Heat" }))).toBe("Heat");
    expect(itemTitle(item({ kind: "audio", title: "Song", year: 2001 }))).toBe("Song");
  });
});

describe("episodeCode", () => {
  it("pads season and episode", () => {
    expect(episodeCode(item({ show: "S", season: 2, episode: 5 }))).toBe("S02E05");
    expect(episodeCode(item({ show: "S", season: 12, episode: 103 }))).toBe("S12E103");
  });
  it("leaves out an unknown season, and is empty for anything that is not an episode", () => {
    expect(episodeCode(item({ show: "S", episode: 5 }))).toBe("E05");
    expect(episodeCode(item({ show: "", episode: 5 }))).toBe("");
    expect(episodeCode(item({ show: "S", episode: 0 }))).toBe("");
  });
});
