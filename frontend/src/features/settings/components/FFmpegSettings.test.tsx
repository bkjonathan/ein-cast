import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Server rendering sees a store's initial state only, so the hooks are replaced
// by state the test controls (as in TvSettings.test.tsx).
const mem = vi.hoisted(() => ({
  settings: { settings: null } as { settings: Record<string, unknown> | null },
  ffmpeg: { status: null } as { status: Record<string, unknown> | null },
}));
vi.mock("../store", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
}));
vi.mock("../hooks/useFFmpeg", () => ({
  useFFmpeg: () => ({
    status: mem.ffmpeg.status, pending: false, error: null,
    recheck: vi.fn(), locate: vi.fn(), useAutomatic: vi.fn(),
  }),
}));
vi.mock("../hooks/useSettingsCommands", () => ({
  useSettingsCommands: () => ({ save: vi.fn(), rotate: vi.fn(), pending: false, error: null }),
}));

import { FFmpegSettings } from "./FFmpegSettings";

const set = (over: Record<string, unknown> = {}) => {
  mem.settings.settings = {
    port: 8765, name: "Den", requireToken: false, dlna: false, autoStart: false, keepAwake: true,
    openAtLogin: false, hideOnClose: false, ffmpegPath: "", hardwareEncoding: true, ...over,
  };
};

beforeEach(() => {
  mem.settings.settings = null;
  mem.ffmpeg.status = null;
});

describe("FFmpegSettings hardware acceleration", () => {
  it("draws nothing until the settings and ffmpeg's status are known", () => {
    expect(renderToStaticMarkup(<FFmpegSettings />)).toBe("");
    set();
    expect(renderToStaticMarkup(<FFmpegSettings />)).toBe("");
  });

  it("offers the switch, on, once ffmpeg is found", () => {
    set();
    mem.ffmpeg.status = { available: true, path: "/opt/homebrew/bin/ffmpeg", chosen: "", installCommand: "" };
    const html = renderToStaticMarkup(<FFmpegSettings />);
    expect(html).toMatch(/Use hardware acceleration/);
    expect(html).toMatch(/aria-checked="true"/);
    expect(html).toMatch(/uses the processor instead/);
  });

  it("shows it off when the person turned it off", () => {
    set({ hardwareEncoding: false });
    mem.ffmpeg.status = { available: true, path: "/x/ffmpeg", chosen: "", installCommand: "" };
    expect(renderToStaticMarkup(<FFmpegSettings />)).toMatch(/aria-checked="false"/);
  });

  it("does not offer a switch for something that cannot run", () => {
    set();
    mem.ffmpeg.status = { available: false, path: "", chosen: "", installCommand: "brew install ffmpeg" };
    expect(renderToStaticMarkup(<FFmpegSettings />)).not.toMatch(/hardware acceleration/);
  });
});
