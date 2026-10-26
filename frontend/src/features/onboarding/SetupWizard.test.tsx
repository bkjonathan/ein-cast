import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { LogLine } from "@/shared/bridge";

// Server rendering sees a zustand store's initial state only, so the stores the
// steps read are replaced by selectors over state the test controls.
const mem = vi.hoisted(() => ({
  libraries: [] as { id: string; scanning: boolean; total: number; error: string; root: string }[],
  server: { status: null } as { status: Record<string, unknown> | null },
  settings: { settings: { requireToken: false, accessToken: "" } },
  logs: { lines: [] as LogLine[] },
}));
vi.mock("@/features/library", () => ({
  useLibraryStore: <T,>(select: (s: { libraries: typeof mem.libraries }) => T): T => select({ libraries: mem.libraries }),
  isFirstScan: (l: typeof mem.libraries) => l.some((x) => x.scanning) && l.every((x) => x.total === 0 && x.error === ""),
  useLibraryActions: () => ({ add: vi.fn(), busy: false, error: null }),
}));
vi.mock("@/features/server", () => ({
  useServerStore: <T,>(select: (s: typeof mem.server) => T): T => select(mem.server),
  useServerControls: () => ({ start: vi.fn(), pending: false, commandError: null }),
}));
vi.mock("@/features/settings", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
  usePlatform: () => "darwin",
}));
vi.mock("@/features/logs", () => {
  const useLogStore = <T,>(select: (s: typeof mem.logs) => T): T => select(mem.logs);
  useLogStore.getState = () => mem.logs;
  return { useLogStore };
});

import { ConnectStep } from "./ConnectStep";
import { SetupWizard } from "./SetupWizard";

const library = (over = {}) => ({ id: "l1", scanning: false, total: 3, error: "", root: "/media", ...over });
const running = { state: "running", name: "Den", port: 8765, localUrl: "http://127.0.0.1:8765", lanUrls: ["http://192.168.1.5:8765"], nameUrls: [], discovery: [], error: "" };

beforeEach(() => {
  mem.libraries = [];
  mem.server.status = null;
  mem.settings.settings = { requireToken: false, accessToken: "" };
  mem.logs.lines = [];
});

describe("SetupWizard", () => {
  const render = () => renderToStaticMarkup(<SetupWizard onDone={() => undefined} />);

  it("starts with the folder, as step 1 of 3, and always offers the way out", () => {
    const html = render();
    expect(html).toMatch(/Choose Media Folder/);
    expect(html).toMatch(/aria-current="step"[^>]*>.*?1.*?Choose your media/s);
    expect(html).toMatch(/Skip setup/);
  });

  it("shows the scan while the first folder is read", () => {
    mem.libraries = [library({ scanning: true, total: 0 })];
    expect(render()).toMatch(/Scanning your library/);
  });

  it("moves on to sharing once a library exists, and offers to start when the server is not running", () => {
    mem.libraries = [library()];
    mem.server.status = { ...running, state: "idle", lanUrls: [] };
    const html = render();
    expect(html).toMatch(/Start sharing/);
    expect(html).not.toMatch(/Choose Media Folder/);
  });

  it("shows the code and the address for the home network when the server runs", () => {
    mem.libraries = [library()];
    mem.server.status = running;
    const html = render();
    expect(html).toMatch(/You&#x27;re sharing|You're sharing/);
    expect(html).toMatch(/http:\/\/192\.168\.1\.5:8765/);
    expect(html).toMatch(/QR code for/);
  });

  it("puts the access token in the code, and says so", () => {
    mem.libraries = [library()];
    mem.server.status = running;
    mem.settings.settings = { requireToken: true, accessToken: "TOK" };
    const html = render();
    expect(html).toMatch(/t=TOK/);
    expect(html).toMatch(/includes your access token/);
  });

  it("explains when there is no network instead of drawing a code that leads nowhere", () => {
    mem.libraries = [library()];
    mem.server.status = { ...running, lanUrls: [] };
    const html = render();
    expect(html).toMatch(/No local network found/);
    expect(html).not.toMatch(/QR code for/);
  });
});

describe("ConnectStep", () => {
  const render = () => renderToStaticMarkup(<ConnectStep onDone={() => undefined} />);
  const request = (seq: number, client: string): LogLine => ({
    seq, time: 0, level: "info", source: "http", message: "request",
    attrs: [{ key: "client_ip", value: client }, { key: "status", value: "200" }, { key: "path", value: "/" }],
  });

  it("waits, with the firewall note, until a device connects", () => {
    const html = render();
    expect(html).toMatch(/Waiting for a device/);
    expect(html).toMatch(/macOS asks|If macOS asks/);
    expect(html).toMatch(/Finish without waiting/);
  });

  it("only counts a device that connected after the step opened", () => {
    mem.logs.lines = [request(7, "192.168.1.20")]; // already in the log when the step opens
    expect(render()).toMatch(/Waiting for a device/);
  });
});
