import { beforeEach, describe, expect, it } from "vitest";
import type { LibraryStatus, LibrarySummary } from "@/shared/bridge";
import { idleStatus, useLibraryStore } from "./store";

const summary = (root: string): LibrarySummary => ({
  root,
  total: 1,
  totalSize: 10,
  counts: { video: 1, audio: 0, image: 0 },
});
const status = (over: Partial<LibraryStatus> = {}): LibraryStatus => ({ ...idleStatus, ...over });

beforeEach(() => useLibraryStore.setState({ summary: null, status: null, hydrated: false }));

describe("library store", () => {
  it("starts empty and not hydrated", () => {
    expect(useLibraryStore.getState()).toMatchObject({ summary: null, status: null, hydrated: false });
  });

  it("hydrate(null, status) marks hydrated without inventing a library", () => {
    useLibraryStore.getState().hydrate(null, status());
    expect(useLibraryStore.getState()).toMatchObject({ summary: null, hydrated: true });
  });

  it("hydrate adopts Go's existing library and status", () => {
    useLibraryStore.getState().hydrate(summary("/a"), status({ root: "/a" }));
    expect(useLibraryStore.getState().summary?.root).toBe("/a");
    expect(useLibraryStore.getState().status?.root).toBe("/a");
  });

  it("applyScanned replaces the library", () => {
    useLibraryStore.getState().applyScanned(summary("/a"));
    useLibraryStore.getState().applyScanned(summary("/b"));
    expect(useLibraryStore.getState().summary?.root).toBe("/b");
  });

  it("events that beat the startup queries are not overwritten by them", () => {
    useLibraryStore.getState().applyScanned(summary("/new"));
    useLibraryStore.getState().applyStatus(status({ root: "/new" }));
    useLibraryStore.getState().hydrate(summary("/stale"), status({ scanning: true, root: "/stale" }));

    expect(useLibraryStore.getState().summary?.root).toBe("/new");
    expect(useLibraryStore.getState().status).toEqual(status({ root: "/new" }));
  });

  it("keeps a failed restore's reason, so the first-run screen can show it", () => {
    useLibraryStore.getState().hydrate(null, status({ root: "/gone", error: "That folder no longer exists." }));
    expect(useLibraryStore.getState().status?.error).toMatch(/no longer exists/);
    expect(useLibraryStore.getState().summary).toBeNull();
  });

  it("a later status event clears the error", () => {
    useLibraryStore.getState().applyStatus(status({ error: "boom" }));
    useLibraryStore.getState().applyStatus(status({ scanning: true }));
    expect(useLibraryStore.getState().status).toEqual(status({ scanning: true }));
  });
});
