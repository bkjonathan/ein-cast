import { describe, expect, it } from "vitest";
import { formFrom, inputFrom, isDirty, needsRestart, parsePort, smartTvsInsteadOfToken, tvNeedsRestart } from "./dirty";

const saved = { port: 8765, name: "EinCast" };

describe("isDirty", () => {
  it("is false for the saved values, and ignores surrounding spaces", () => {
    expect(isDirty(formFrom(saved), saved)).toBe(false);
    expect(isDirty({ port: " 8765 ", name: "  EinCast " }, saved)).toBe(false);
  });

  it.each([
    [{ port: "9000", name: "EinCast" }],
    [{ port: "8765", name: "Den" }],
    [{ port: "", name: "EinCast" }],
  ])("is true when %j differs", (form) => {
    expect(isDirty(form, saved)).toBe(true);
  });
});

describe("needsRestart", () => {
  const running = (over = {}) => ({ state: "running", port: 8765, name: "EinCast", ...over });

  it("is false when the server is stopped, whatever was saved", () => {
    expect(needsRestart({ state: "idle", port: 0, name: "EinCast" }, { port: 9000, name: "x" })).toBe(false);
    expect(needsRestart(null, saved)).toBe(false);
  });

  it("is false while running with the saved values", () => {
    expect(needsRestart(running(), saved)).toBe(false);
  });

  it("is true when the running port or name differs from the saved one", () => {
    expect(needsRestart(running({ port: 8000 }), saved)).toBe(true);
    expect(needsRestart(running({ name: "Old" }), saved)).toBe(true);
  });
});

describe("tvNeedsRestart", () => {
  const running = (discovery: string[], state = "running") => ({ state, discovery });

  it("is false when nothing runs, whatever was saved", () => {
    expect(tvNeedsRestart(null, { dlna: true })).toBe(false);
    expect(tvNeedsRestart(running([], "idle"), { dlna: true })).toBe(false);
  });

  it("is false when the running server already matches the setting", () => {
    expect(tvNeedsRestart(running(["mdns", "dlna"]), { dlna: true })).toBe(false);
    expect(tvNeedsRestart(running(["mdns"]), { dlna: false })).toBe(false);
  });

  it("is true when the setting was switched on or off after the server started", () => {
    expect(tvNeedsRestart(running(["mdns"]), { dlna: true })).toBe(true);
    expect(tvNeedsRestart(running(["mdns", "dlna"]), { dlna: false })).toBe(true);
  });
});

describe("parsePort", () => {
  it.each([
    ["8765", 8765],
    [" 9000 ", 9000],
    ["", 0],
    ["abc", 0],
    ["80.5", 0],
    ["1e3", 1000],
  ])("%j → %d", (text, expected) => {
    expect(parsePort(text)).toBe(expected);
  });
});

describe("inputFrom", () => {
  const all = {
    port: 8765, name: "EinCast", requireToken: true, dlna: false,
    autoStart: true, keepAwake: true, openAtLogin: false, hideOnClose: true,
  };

  it("sends every field back, because Go replaces them all on each save", () => {
    expect(inputFrom(all)).toStrictEqual(all);
  });

  it("changes only what it is told to, and drops fields that are not editable", () => {
    const withExtras = { ...all, accessToken: "secret", tokenPersisted: true, ffmpegPath: "/x" };
    const got = inputFrom(withExtras, { keepAwake: false });
    expect(got).toStrictEqual({ ...all, keepAwake: false });
    expect(Object.keys(got)).not.toContain("accessToken"); // a token must never ride back
  });
});

describe("smartTvsInsteadOfToken", () => {
  it("makes the exclusive choice in one save and leaves everything else alone", () => {
    const saved = {
      port: 9000, name: "Den", requireToken: true, dlna: false,
      autoStart: true, keepAwake: false, openAtLogin: true, hideOnClose: false,
    };
    expect(smartTvsInsteadOfToken(saved)).toStrictEqual({ ...saved, requireToken: false, dlna: true });
  });
});
