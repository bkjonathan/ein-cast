import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UpdateStatus } from "@/shared/bridge";

// Server rendering sees a store's initial state only, so the stores are replaced
// by state the test controls (as in FFmpegSettings.test.tsx).
const mem = vi.hoisted(() => ({
  settings: { settings: null } as { settings: Record<string, unknown> | null },
  updates: { status: null } as { status: UpdateStatus | null },
}));
vi.mock("../store", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
}));
vi.mock("@/features/updates", () => ({
  useUpdatesStore: <T,>(select: (s: typeof mem.updates) => T): T => select(mem.updates),
  useUpdateCommands: () => ({ check: vi.fn(), openPage: vi.fn(), error: null }),
}));
vi.mock("../hooks/useSettingsCommands", () => ({
  useSettingsCommands: () => ({ save: vi.fn(), rotate: vi.fn(), pending: false, error: null }),
}));

import { UpdateSettings } from "./UpdateSettings";

const setSettings = (over: Record<string, unknown> = {}) => {
  mem.settings.settings = {
    port: 8765, name: "Den", requireToken: false, dlna: false, autoStart: false, keepAwake: true,
    openAtLogin: false, hideOnClose: false, ffmpegPath: "", hardwareEncoding: true, checkForUpdates: true, ...over,
  };
};

const status = (over: Partial<UpdateStatus> = {}): UpdateStatus => ({
  current: "1.1.1", checking: false, available: false, latest: null, checkedAt: 0, error: "", ...over,
});

const release = (over: Partial<NonNullable<UpdateStatus["latest"]>> = {}) => ({
  version: "1.1.2", name: "EinCast 1.1.2", notes: "- fixes", publishedAt: Date.UTC(2026, 10, 1, 12),
  assets: [
    { name: "eincast-1.1.2-macos-universal.zip", size: 5 * 1024 * 1024, sha256: "ab".repeat(32) },
    { name: "eincast-1.1.2-other.zip", size: 1024, sha256: "" },
  ],
  ...over,
});

beforeEach(() => {
  mem.settings.settings = null;
  mem.updates.status = null;
});

describe("UpdateSettings", () => {
  it("draws nothing until the settings and the update status are known", () => {
    expect(renderToStaticMarkup(<UpdateSettings />)).toBe("");
    setSettings();
    expect(renderToStaticMarkup(<UpdateSettings />)).toBe("");
  });

  it("says what is running and shows the automatic check on", () => {
    setSettings();
    mem.updates.status = status();
    const html = renderToStaticMarkup(<UpdateSettings />);
    expect(html).toMatch(/You are running version 1\.1\.1/);
    expect(html).toMatch(/Not checked yet/);
    expect(html).toMatch(/aria-checked="true"/);
  });

  it("shows the automatic check off when the person turned it off", () => {
    setSettings({ checkForUpdates: false });
    mem.updates.status = status();
    expect(renderToStaticMarkup(<UpdateSettings />)).toMatch(/aria-checked="false"/);
  });

  it("says so when the newest version is already installed", () => {
    setSettings();
    mem.updates.status = status({ checkedAt: Date.UTC(2026, 10, 1, 12), latest: release({ version: "1.1.1" }) });
    const html = renderToStaticMarkup(<UpdateSettings />);
    expect(html).toMatch(/You have the newest version/);
    expect(html).not.toMatch(/is available/);
  });

  it("offers the new version with its files, sizes and checksums", () => {
    setSettings();
    mem.updates.status = status({ available: true, checkedAt: 1, latest: release() });
    const html = renderToStaticMarkup(<UpdateSettings />);
    expect(html).toMatch(/Version 1\.1\.2 is available/);
    expect(html).toMatch(/eincast-1\.1\.2-macos-universal\.zip/);
    expect(html).toMatch(/5\.0 MB/);
    expect(html).toContain("SHA-256 " + "ab".repeat(32));
    expect(html).toMatch(/No checksum was published for this file/);
    expect(html).toMatch(/Open download page/);
  });

  it("shows release notes as text, never as markup", () => {
    setSettings();
    mem.updates.status = status({ available: true, latest: release({ notes: '<img src=x onerror="alert(1)"><b>hi</b>' }) });
    const html = renderToStaticMarkup(<UpdateSettings />);
    expect(html).not.toContain("<img");
    expect(html).not.toContain("<b>");
    expect(html).toContain("&lt;img src=x");
  });

  it("shows a failed check, and keeps the older notice beside it", () => {
    setSettings();
    mem.updates.status = status({ available: true, latest: release(), error: "GitHub could not be reached: no such host" });
    const html = renderToStaticMarkup(<UpdateSettings />);
    expect(html).toMatch(/GitHub could not be reached/);
    expect(html).toMatch(/Version 1\.1\.2 is available/);
  });

  it("disables the button and says so while checking", () => {
    setSettings();
    mem.updates.status = status({ checking: true });
    const html = renderToStaticMarkup(<UpdateSettings />);
    expect(html).toMatch(/Checking…/);
    expect(html).toMatch(/disabled/);
  });

  it("explains a build from source in the window's own words, not Go's English", () => {
    setSettings();
    mem.updates.status = status({ current: "dev", error: "This build was made from source and has no version number." });
    const html = renderToStaticMarkup(<UpdateSettings />);
    expect(html).toMatch(/made from source and has no version number, so there is nothing to compare/);
    expect(html).not.toMatch(/role="alert"/);
  });
});
