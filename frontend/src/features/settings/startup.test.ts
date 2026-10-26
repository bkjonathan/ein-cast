import { describe, expect, it } from "vitest";
import { networkHints } from "./network";
import { hideOnCloseHelp, startupRows } from "./startup";

describe("hideOnCloseHelp", () => {
  it("says where the app lives and how to really quit, per platform", () => {
    expect(hideOnCloseHelp("darwin")).toMatch(/Dock/);
    expect(hideOnCloseHelp("darwin")).toMatch(/⌘Q/);
    expect(hideOnCloseHelp("windows")).toMatch(/notification area/);
    expect(hideOnCloseHelp("windows")).toMatch(/Quit/);
  });

  it("always says the change waits for the next start, because the window option is read once", () => {
    for (const p of ["darwin", "windows", ""]) expect(hideOnCloseHelp(p)).toMatch(/next time EinCast opens/);
  });
});

describe("startupRows", () => {
  it("covers every plain switch once, so none can be saved without a way to see it", () => {
    expect(startupRows.map((r) => r.key)).toStrictEqual(["autoStart", "keepAwake", "openAtLogin"]);
    for (const r of startupRows) expect(r.help.length).toBeGreaterThan(20);
  });
});

describe("networkHints", () => {
  it("names the ports that matter and the firewall prompt", () => {
    const text = networkHints.map((h) => h.text).join(" ");
    expect(text).toMatch(/1900/);
    expect(text).toMatch(/5353/);
    expect(text).toMatch(/Private networks/);
  });
});
