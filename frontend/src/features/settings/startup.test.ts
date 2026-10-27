import { describe, expect, it } from "vitest";
import { translate } from "@/shared/i18n";
import { networkHints } from "./network";
import { hideOnCloseHelp, startupRows } from "./startup";

const en = (key: Parameters<typeof translate>[1]) => translate("en", key);

describe("hideOnCloseHelp", () => {
  it("says where the app lives and how to really quit, per platform", () => {
    expect(en(hideOnCloseHelp("darwin"))).toMatch(/Dock/);
    expect(en(hideOnCloseHelp("darwin"))).toMatch(/⌘Q/);
    expect(en(hideOnCloseHelp("windows"))).toMatch(/notification area/);
    expect(en(hideOnCloseHelp("windows"))).toMatch(/Quit/);
  });

  it("always says the change waits for the next start, because the window option is read once", () => {
    for (const p of ["darwin", "windows", ""]) expect(en(hideOnCloseHelp(p))).toMatch(/next time EinCast opens/);
  });
});

describe("startupRows", () => {
  it("covers every plain switch once, so none can be saved without a way to see it", () => {
    expect(startupRows.map((r) => r.key)).toStrictEqual(["autoStart", "keepAwake", "openAtLogin"]);
    for (const r of startupRows) expect(en(r.helpKey).length).toBeGreaterThan(20);
  });
});

describe("networkHints", () => {
  it("names the ports that matter and the firewall prompt", () => {
    const text = networkHints.map((h) => en(h.textKey)).join(" ");
    expect(text).toMatch(/1900/);
    expect(text).toMatch(/5353/);
    expect(text).toMatch(/Private networks/);
  });
});
