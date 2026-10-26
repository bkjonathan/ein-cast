import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Server rendering sees a store's initial state only, so the store hook is a
// selector over state the test controls (as in TvSettings.test).
const mem = vi.hoisted(() => ({
  settings: { settings: null } as { settings: { port: number } | null },
}));
vi.mock("../store", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
}));

import { PortSettings } from "./PortSettings";

beforeEach(() => {
  mem.settings.settings = null;
});

describe("PortSettings", () => {
  it("draws nothing until the settings are known", () => {
    expect(renderToStaticMarkup(<PortSettings />)).toBe("");
  });

  it("starts on the saved port and only offers to look, never to end anything yet", () => {
    mem.settings.settings = { port: 8765 };
    const html = renderToStaticMarkup(<PortSettings />);
    expect(html).toMatch(/value="8765"/);
    expect(html).toMatch(/Check/);
    expect(html).not.toMatch(/End process/);
  });
});
