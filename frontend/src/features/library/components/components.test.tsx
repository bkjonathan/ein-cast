import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { LibraryState } from "@/shared/bridge";

// Server rendering sees a zustand store's initial state only, so the store hook
// is replaced by a selector over state the test controls (as in the remote and
// Smart TV panels' tests). Logic lives in derive.test.ts and store.test.ts.
const mem = vi.hoisted(() => ({ libraries: [] as LibraryState[] }));
vi.mock("../store", () => ({
  useLibraryStore: <T,>(select: (s: typeof mem) => T): T => select(mem),
}));
vi.mock("../hooks/useLibraryActions", () => ({
  useLibraryActions: () => ({
    add: async () => undefined,
    rescan: async () => undefined,
    rescanAll: async () => undefined,
    remove: async () => undefined,
    rename: async () => true,
    busy: false,
    error: null,
  }),
}));

import { LibraryCard } from "./LibraryCard";
import { LibraryManager } from "./LibraryManager";
import { LibraryStats } from "./LibraryStats";

const lib = (id: string, over: Partial<LibraryState> = {}): LibraryState => ({
  id,
  name: `Library ${id}`,
  root: `/media/${id}`,
  total: 3,
  totalSize: 2048,
  counts: { video: 2, audio: 1, image: 0 },
  scanning: false,
  error: "",
  watching: true,
  ...over,
});

beforeEach(() => {
  mem.libraries = [];
});

describe("LibraryCard", () => {
  it("shows the name, folder, contents and that the folder is watched", () => {
    const html = renderToStaticMarkup(<LibraryCard library={lib("a")} />);
    expect(html).toMatch(/Library a/);
    expect(html).toMatch(/\/media\/a/);
    expect(html).toMatch(/2 videos · 1 audio · 0 images · 2\.0 KB/);
    expect(html).toMatch(/Watching this folder/);
    expect(html).not.toMatch(/role="alert"/);
  });

  it("says changes need a rescan when the folder is not watched", () => {
    const html = renderToStaticMarkup(<LibraryCard library={lib("a", { watching: false })} />);
    expect(html).toMatch(/Use Rescan after adding or removing files/);
  });

  it("explains a failure, and mentions the kept scan only if there is one", () => {
    const kept = renderToStaticMarkup(<LibraryCard library={lib("a", { error: "That folder no longer exists." })} />);
    expect(kept).toMatch(/role="alert"/);
    expect(kept).toMatch(/no longer exists\. Showing the last successful scan/);

    const none = renderToStaticMarkup(<LibraryCard library={lib("a", { total: 0, error: "That folder no longer exists." })} />);
    expect(none).toMatch(/no longer exists/);
    expect(none).not.toMatch(/Showing the last successful scan/);
  });

  it("shows progress while scanning", () => {
    expect(renderToStaticMarkup(<LibraryCard library={lib("a", { scanning: true })} />)).toMatch(/Scanning…/);
  });

  it("has labelled controls for rename and remove", () => {
    const html = renderToStaticMarkup(<LibraryCard library={lib("a")} />);
    expect(html).toMatch(/aria-label="Rename Library a"/);
    expect(html).toMatch(/aria-label="Remove Library a"/);
  });
});

describe("LibraryManager", () => {
  it("lists every library and offers to add one", () => {
    mem.libraries = [lib("a"), lib("b")];
    const html = renderToStaticMarkup(<LibraryManager />);
    expect(html).toMatch(/Library a/);
    expect(html).toMatch(/Library b/);
    expect(html).toMatch(/Add folder…/);
  });

  it("offers Rescan all only when there is more than one library", () => {
    mem.libraries = [lib("a")];
    expect(renderToStaticMarkup(<LibraryManager />)).not.toMatch(/Rescan all/);
    mem.libraries = [lib("a"), lib("b")];
    expect(renderToStaticMarkup(<LibraryManager />)).toMatch(/Rescan all/);
  });
});

describe("LibraryStats", () => {
  it("draws nothing without libraries", () => {
    expect(renderToStaticMarkup(<LibraryStats />)).toBe("");
  });

  it("adds up the libraries", () => {
    mem.libraries = [lib("a"), lib("b", { counts: { video: 1, audio: 0, image: 4 }, total: 5, totalSize: 1024 })];
    const html = renderToStaticMarkup(<LibraryStats />);
    expect(html).toMatch(/2 folders/);
    expect(html).toMatch(/Videos<\/dt><dd[^>]*>3</); // 2 + 1
    expect(html).toMatch(/Images<\/dt><dd[^>]*>4</);
    expect(html).toMatch(/3\.0 KB/);
  });

  it("lists the libraries only when it helps: several, or one with a problem", () => {
    mem.libraries = [lib("a")];
    expect(renderToStaticMarkup(<LibraryStats />)).not.toMatch(/aria-label="Libraries"/);

    mem.libraries = [lib("a"), lib("b")];
    expect(renderToStaticMarkup(<LibraryStats />)).toMatch(/aria-label="Libraries"/);

    mem.libraries = [lib("a", { error: "That folder no longer exists." })];
    const html = renderToStaticMarkup(<LibraryStats />);
    expect(html).toMatch(/aria-label="Libraries"/);
    expect(html).toMatch(/no longer exists/);
  });

  it("says so when nothing was found", () => {
    mem.libraries = [lib("a", { total: 0, totalSize: 0, counts: { video: 0, audio: 0, image: 0 } })];
    expect(renderToStaticMarkup(<LibraryStats />)).toMatch(/No supported media found yet/);
  });
});
