import { describe, expect, it } from "vitest";
import type { LibraryState } from "@/shared/bridge";
import { effectiveLibrary, isFirstScan, nameById, totalsOf } from "./derive";

const lib = (id: string, over: Partial<LibraryState> = {}): LibraryState => ({
  id,
  name: `Lib ${id}`,
  root: `/m/${id}`,
  total: 0,
  totalSize: 0,
  counts: { video: 0, audio: 0, image: 0 },
  scanning: false,
  error: "",
  watching: false,
  ...over,
});

describe("totalsOf", () => {
  it("adds up every library", () => {
    const got = totalsOf([
      lib("a", { total: 3, totalSize: 30, counts: { video: 2, audio: 1, image: 0 } }),
      lib("b", { total: 2, totalSize: 5, counts: { video: 0, audio: 0, image: 2 } }),
    ]);
    expect(got).toEqual({ total: 5, totalSize: 35, counts: { video: 2, audio: 1, image: 2 } });
  });

  it("is all zeros for no libraries, and does not touch its input", () => {
    expect(totalsOf([])).toEqual({ total: 0, totalSize: 0, counts: { video: 0, audio: 0, image: 0 } });
    const a = lib("a", { total: 1, counts: { video: 1, audio: 0, image: 0 } });
    totalsOf([a, a]);
    expect(a.counts.video).toBe(1);
  });
});

describe("isFirstScan", () => {
  it("is true while the only library is being scanned for the first time", () => {
    expect(isFirstScan([lib("a", { scanning: true })])).toBe(true);
  });

  it("is false once something has files, or has failed, or nothing is scanning", () => {
    expect(isFirstScan([lib("a", { scanning: true, total: 4 })])).toBe(false);
    expect(isFirstScan([lib("a", { scanning: true }), lib("b", { total: 1 })])).toBe(false);
    expect(isFirstScan([lib("a", { scanning: true }), lib("b", { error: "gone" })])).toBe(false);
    expect(isFirstScan([lib("a")])).toBe(false);
    expect(isFirstScan([])).toBe(false);
  });

  it("an empty folder that finished scanning is not 'first scan' any more", () => {
    expect(isFirstScan([lib("a", { scanning: false, total: 0 })])).toBe(false);
  });
});

describe("nameById", () => {
  it("maps IDs to names", () => {
    expect(nameById([lib("a"), lib("b")]).get("b")).toBe("Lib b");
    expect(nameById([]).size).toBe(0);
  });
});

describe("effectiveLibrary", () => {
  it("keeps a choice that still exists, and all", () => {
    expect(effectiveLibrary("a", [lib("a"), lib("b")])).toBe("a");
    expect(effectiveLibrary("all", [lib("a")])).toBe("all");
  });

  it("falls back to all when the chosen library was removed", () => {
    expect(effectiveLibrary("gone", [lib("a")])).toBe("all");
    expect(effectiveLibrary("a", [])).toBe("all");
  });
});
