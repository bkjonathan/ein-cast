import { describe, expect, it } from "vitest";
import { formFrom, isDirty, needsRestart, parsePort } from "./dirty";

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
