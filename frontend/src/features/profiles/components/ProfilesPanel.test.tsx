import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { LibraryState, Profile } from "@/shared/bridge";

// Server rendering sees a zustand store's initial state only, so the stores are
// replaced by selectors over state the test controls.
const mem = vi.hoisted(() => ({
  profiles: { profiles: null as Profile[] | null },
  library: { libraries: [] as LibraryState[] },
}));
vi.mock("../store", () => ({
  useProfilesStore: <T,>(select: (s: typeof mem.profiles) => T): T => select(mem.profiles),
}));
vi.mock("@/features/library", () => ({
  useLibraryStore: <T,>(select: (s: typeof mem.library) => T): T => select(mem.library),
}));

import { ProfilesPanel } from "./ProfilesPanel";

const profile = (over: Partial<Profile> = {}): Profile => ({
  id: "p", name: "Everyone", color: "blue", allLibraries: true, libraries: [], kinds: [], hiddenFolders: [], hasPin: false, ...over,
});

beforeEach(() => {
  mem.profiles.profiles = null;
  mem.library.libraries = [
    { id: "a", name: "Cartoons", root: "/c", total: 0, totalSize: 0, counts: { video: 0, audio: 0, image: 0 }, scanning: false, error: "", watching: true, hiddenFromTVs: false },
  ];
});

describe("ProfilesPanel", () => {
  it("draws nothing until Go has reported", () => {
    expect(renderToStaticMarkup(<ProfilesPanel />)).toBe("");
  });

  it("invites adding a profile when there is only the default one, with no warning", () => {
    mem.profiles.profiles = [profile()];
    const html = renderToStaticMarkup(<ProfilesPanel />);
    expect(html).toMatch(/Add a profile for the kids/);
    expect(html).not.toMatch(/can see everything and has no PIN/);
  });

  it("lists each profile with what it can see and whether it has a PIN", () => {
    mem.profiles.profiles = [
      profile({ hasPin: true }),
      profile({ id: "k", name: "Kids", allLibraries: false, libraries: ["a"], kinds: ["video"], hiddenFolders: ["18*"] }),
    ];
    const html = renderToStaticMarkup(<ProfilesPanel />);
    expect(html).toMatch(/Everyone/);
    expect(html).toMatch(/· PIN/);
    expect(html).toMatch(/Cartoons · Videos only · hides 1 folder/);
  });

  it("warns when a profile that sees everything has no PIN next to a restricted one", () => {
    mem.profiles.profiles = [profile(), profile({ id: "k", name: "Kids", kinds: ["video"] })];
    expect(renderToStaticMarkup(<ProfilesPanel />)).toMatch(/“Everyone” can see everything and has no PIN/);
  });

  it("stops offering to add at the limit", () => {
    mem.profiles.profiles = Array.from({ length: 12 }, (_, i) => profile({ id: `p${i}`, name: `P${i}`, hasPin: true }));
    const html = renderToStaticMarkup(<ProfilesPanel />);
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>[^<]*<svg[^>]*>.*?<\/svg>\s*Add profile/s);
  });
});
