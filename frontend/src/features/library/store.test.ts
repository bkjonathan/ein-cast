import { beforeEach, describe, expect, it } from "vitest";
import type { LibrarySummary } from "@/shared/bridge";
import { useLibraryStore } from "./store";

const summary = (root: string): LibrarySummary => ({
  root,
  total: 1,
  totalSize: 10,
  counts: { video: 1, audio: 0, image: 0 },
});

beforeEach(() => useLibraryStore.setState({ summary: null, hydrated: false }));

describe("library store", () => {
  it("starts empty and not hydrated", () => {
    expect(useLibraryStore.getState()).toMatchObject({ summary: null, hydrated: false });
  });

  it("hydrate(null) marks hydrated without inventing a library", () => {
    useLibraryStore.getState().hydrate(null);
    expect(useLibraryStore.getState()).toMatchObject({ summary: null, hydrated: true });
  });

  it("hydrate(summary) adopts Go's existing library", () => {
    useLibraryStore.getState().hydrate(summary("/a"));
    expect(useLibraryStore.getState().summary?.root).toBe("/a");
  });

  it("applyScanned replaces the library", () => {
    useLibraryStore.getState().applyScanned(summary("/a"));
    useLibraryStore.getState().applyScanned(summary("/b"));
    expect(useLibraryStore.getState().summary?.root).toBe("/b");
  });

  it("an event that beats the startup query is not overwritten by it", () => {
    useLibraryStore.getState().applyScanned(summary("/new"));
    useLibraryStore.getState().hydrate(summary("/stale"));
    expect(useLibraryStore.getState().summary?.root).toBe("/new");
  });
});
