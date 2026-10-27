import { beforeEach, describe, expect, it } from "vitest";
import type { LibraryState } from "@/shared/bridge";
import { useLibraryStore } from "./store";

const lib = (id: string, over: Partial<LibraryState> = {}): LibraryState => ({
  id,
  name: id,
  root: `/media/${id}`,
  total: 1,
  totalSize: 10,
  counts: { video: 1, audio: 0, image: 0 },
  scanning: false,
  error: "",
  watching: false,
  hiddenFromTVs: false,
  ...over,
});

beforeEach(() =>
  useLibraryStore.setState({
    libraries: [],
    hydrated: false,
    heard: false,
    items: [],
    kind: "all",
    library: "all",
    query: "",
    sort: "name",
  }),
);

describe("library store", () => {
  it("starts empty and not hydrated", () => {
    expect(useLibraryStore.getState()).toMatchObject({ libraries: [], hydrated: false, heard: false });
  });

  it("hydrate([]) marks hydrated without inventing a library", () => {
    useLibraryStore.getState().hydrate([]);
    expect(useLibraryStore.getState()).toMatchObject({ libraries: [], hydrated: true });
  });

  it("hydrate adopts Go's libraries", () => {
    useLibraryStore.getState().hydrate([lib("a"), lib("b")]);
    expect(useLibraryStore.getState().libraries.map((l) => l.id)).toEqual(["a", "b"]);
  });

  it("apply replaces the whole list: a removed library disappears", () => {
    useLibraryStore.getState().apply([lib("a"), lib("b")]);
    useLibraryStore.getState().apply([lib("b")]);
    expect(useLibraryStore.getState().libraries.map((l) => l.id)).toEqual(["b"]);
  });

  it("apply also hydrates: an event is an answer", () => {
    useLibraryStore.getState().apply([lib("a")]);
    expect(useLibraryStore.getState()).toMatchObject({ hydrated: true, heard: true });
  });

  it("events that beat the startup query are not overwritten by it", () => {
    useLibraryStore.getState().apply([lib("new", { scanning: true })]);
    useLibraryStore.getState().hydrate([lib("stale")]);

    expect(useLibraryStore.getState().libraries.map((l) => l.id)).toEqual(["new"]);
    expect(useLibraryStore.getState().hydrated).toBe(true);
  });

  it("a later event clears an earlier error", () => {
    useLibraryStore.getState().apply([lib("a", { error: "That folder no longer exists." })]);
    useLibraryStore.getState().apply([lib("a", { scanning: true })]);
    expect(useLibraryStore.getState().libraries[0]).toMatchObject({ error: "", scanning: true });
  });

  it("keeps the file list and the browsing choices apart from Go's libraries", () => {
    const item = { id: "a", libraryId: "a", name: "a.mp4", relPath: "a.mp4", title: "a.mp4", year: 0, show: "", season: 0, episode: 0, poster: false, kind: "video" as const, size: 1, modTime: 1 };
    useLibraryStore.getState().setItems([item]);
    useLibraryStore.getState().setKind("audio");
    useLibraryStore.getState().setLibrary("a");
    useLibraryStore.getState().setQuery("x");
    useLibraryStore.getState().setSort("newest");
    useLibraryStore.getState().apply([lib("a")]);
    expect(useLibraryStore.getState()).toMatchObject({
      items: [item],
      kind: "audio",
      library: "a",
      query: "x",
      sort: "newest",
    });
  });
});
