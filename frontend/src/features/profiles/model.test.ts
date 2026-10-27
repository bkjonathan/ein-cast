import { describe, expect, it } from "vitest";
import type { LibraryState, Profile } from "@/shared/bridge";
import { translatorFor } from "@/shared/i18n";

const en = translatorFor("en");
import { blankProfile, describeAccess, foldersText, liveLibraries, openProfileAdvice, parseFolders, pinLooksValid, toggleKind, toggleLibrary } from "./model";

const profile = (over: Partial<Profile> = {}): Profile => ({
  id: "p", name: "Kids", color: "green", allLibraries: true, libraries: [], kinds: [], hiddenFolders: [], hasPin: false, ...over,
});
const lib = (id: string, name: string): LibraryState => ({
  id, name, root: `/${name}`, total: 0, totalSize: 0, counts: { video: 0, audio: 0, image: 0 },
  scanning: false, error: "", watching: true, hiddenFromTVs: false,
});

describe("blankProfile", () => {
  it("starts open to everything, unsaved, and with a colour nobody else has", () => {
    const p = blankProfile(["blue", "green"]);
    expect(p).toMatchObject({ id: "", name: "", allLibraries: true, libraries: [], kinds: [], hiddenFolders: [], hasPin: false });
    expect(["blue", "green"]).not.toContain(p.color);
  });
  it("still gives a colour when every one is taken", () => {
    expect(blankProfile(["blue", "green", "red", "orange", "purple", "pink", "teal", "gray"]).color).not.toBe("");
  });
});

describe("hidden folder names", () => {
  it("parses one per line or comma separated, trimmed, without blanks or repeats", () => {
    expect(parseFolders(" Adults \n\n18+, adults ,Horror\n")).toEqual(["Adults", "18+", "Horror"]);
    expect(parseFolders("")).toEqual([]);
  });
  it("round-trips through the text box", () => {
    expect(parseFolders(foldersText(["a", "b*"]))).toEqual(["a", "b*"]);
  });
});

describe("toggles", () => {
  it("ticks and unticks a library without changing the original", () => {
    const p = profile({ allLibraries: false, libraries: ["a"] });
    expect(toggleLibrary(p, "b").libraries).toEqual(["a", "b"]);
    expect(toggleLibrary(p, "a").libraries).toEqual([]);
    expect(p.libraries).toEqual(["a"]);
  });
  it("keeps kinds in one order, so the same choice saves the same way", () => {
    let p = profile({ kinds: ["image"] });
    p = toggleKind(p, "video");
    expect(p.kinds).toEqual(["video", "image"]);
    expect(toggleKind(p, "video").kinds).toEqual(["image"]);
  });
  it("starts a list of the others when unticking from \"every kind\"", () => {
    expect(toggleKind(profile(), "audio").kinds).toEqual(["video", "image"]);
  });
  it("will not untick the last kind, since an empty list would mean every kind", () => {
    const only = profile({ kinds: ["video"] });
    expect(toggleKind(only, "video")).toBe(only);
  });
});

describe("describeAccess", () => {
  const libs = [lib("a", "Cartoons"), lib("b", "Movies")];
  it.each([
    [profile(), "All libraries"],
    [profile({ allLibraries: false, libraries: ["a"] }), "Cartoons"],
    [profile({ allLibraries: false, libraries: ["a", "b"] }), "Cartoons, Movies"],
    [profile({ allLibraries: false, libraries: [] }), "No libraries"],
    [profile({ allLibraries: false, libraries: ["gone"] }), "No libraries"],
    [profile({ kinds: ["video"] }), "All libraries · Videos only"],
    [profile({ kinds: ["video", "audio"] }), "All libraries · Videos and Music only"],
    [profile({ kinds: ["video", "audio", "image"] }), "All libraries"],
    [profile({ hiddenFolders: ["a"] }), "All libraries · hides 1 folder"],
    [profile({ allLibraries: false, libraries: ["a"], kinds: ["video"], hiddenFolders: ["x", "y"] }), "Cartoons · Videos only · hides 2 folders"],
  ])("%j → %s", (p, text) => {
    expect(describeAccess(p, libs, translatorFor("en"))).toBe(text);
  });
});

describe("openProfileAdvice", () => {
  it("says nothing with a single profile, whatever it can see", () => {
    expect(openProfileAdvice([profile()], translatorFor("en"))).toBeNull();
  });
  it("warns that an unrestricted profile without a PIN is the way round the others", () => {
    const advice = openProfileAdvice([profile({ name: "Everyone" }), profile({ id: "k", name: "Kids", kinds: ["video"] })], en);
    expect(advice).toMatch(/“Everyone” can see everything and has no PIN/);
    expect(advice).not.toMatch(/Kids/);
  });
  it("is satisfied by a PIN, or by a restriction", () => {
    const kids = profile({ id: "k", name: "Kids", kinds: ["video"] });
    expect(openProfileAdvice([profile({ hasPin: true }), kids], en)).toBeNull();
    expect(openProfileAdvice([profile({ hiddenFolders: ["x"] }), kids], en)).toBeNull();
  });
  it("names every open profile", () => {
    expect(openProfileAdvice([profile({ name: "A" }), profile({ id: "b", name: "B" })], en)).toMatch(/“A” and “B” can/);
  });
});

describe("misc", () => {
  it("forgets libraries that no longer exist", () => {
    expect(liveLibraries(profile({ libraries: ["a", "gone"] }), [lib("a", "A")])).toEqual(["a"]);
  });
  it.each([["1234", true], ["123456", true], ["123", false], ["1234567", false], ["12a4", false], ["", false]])("PIN %s → %s", (pin, ok) => {
    expect(pinLooksValid(pin)).toBe(ok);
  });
});
