import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Server rendering sees a zustand store's initial state only, so the store hooks
// are replaced by selectors over state the test controls (as in the remote
// screen's component tests).
const mem = vi.hoisted(() => ({
  settings: { settings: null } as {
    settings: Record<string, unknown> | null;
  },
  server: { status: null } as { status: { state: string; discovery: string[] } | null },
}));
vi.mock("../store", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
}));
vi.mock("@/features/server", () => ({
  useServerStore: <T,>(select: (s: typeof mem.server) => T): T => select(mem.server),
}));

import { TvSettings } from "./TvSettings";

const set = (over: Partial<NonNullable<typeof mem.settings.settings>> = {}) => {
  mem.settings.settings = {
    port: 8765, name: "Den", requireToken: false, dlna: false,
    autoStart: false, keepAwake: true, openAtLogin: false, hideOnClose: false, ...over,
  };
};

beforeEach(() => {
  mem.settings.settings = null;
  mem.server.status = null;
});

describe("TvSettings", () => {
  it("draws nothing until the settings are known", () => {
    expect(renderToStaticMarkup(<TvSettings />)).toBe("");
  });

  it("offers the switch, off, with the warning about an open network", () => {
    set();
    const html = renderToStaticMarkup(<TvSettings />);
    expect(html).toMatch(/aria-checked="false"/);
    expect(html).not.toMatch(/disabled=""/); // the attribute, not the disabled: CSS class
    expect(html).toMatch(/without a password/);
  });

  it("shows it on when enabled", () => {
    set({ dlna: true });
    expect(renderToStaticMarkup(<TvSettings />)).toMatch(/aria-checked="true"/);
  });

  it("with the access token on, still offers the switch and says what turning it on costs", () => {
    set({ requireToken: true });
    const html = renderToStaticMarkup(<TvSettings />);
    expect(html).not.toMatch(/disabled=""/); // the person decides; Go enforces the exclusion
    expect(html).toMatch(/access token is on, and TVs can(&#x27;|')t send it/);
    expect(html).toMatch(/Turning this on turns the token off/);
    expect(html).not.toMatch(/alertdialog/); // the warning appears on click, not before
  });

  it("asks for a restart only when the running server differs from the setting", () => {
    set({ dlna: true });
    mem.server.status = { state: "running", discovery: ["mdns"] };
    expect(renderToStaticMarkup(<TvSettings />)).toMatch(/Stop and start the server/);

    mem.server.status = { state: "running", discovery: ["mdns", "dlna"] };
    expect(renderToStaticMarkup(<TvSettings />)).not.toMatch(/Stop and start the server/);
  });
});
