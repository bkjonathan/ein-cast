import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Server rendering sees a zustand store's initial state only, so the stores are
// replaced by selectors over state the test controls.
const mem = vi.hoisted(() => ({
  server: { status: null } as { status: Record<string, unknown> | null },
  settings: { settings: { port: 8765 } } as { settings: { port: number } | null },
}));
vi.mock("../store", () => ({
  useServerStore: <T,>(select: (s: typeof mem.server) => T): T => select(mem.server),
}));
vi.mock("@/features/settings", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
}));
vi.mock("../hooks/useServerControls", () => ({
  useServerControls: () => ({ start: vi.fn(), stop: vi.fn(), pending: false, commandError: null }),
}));

import { ServerPanel } from "./ServerPanel";

const failed = (error: string) => {
  mem.server.status = { state: "failed", name: "Den", port: 0, error };
};

beforeEach(() => {
  mem.server.status = null;
  mem.settings.settings = { port: 8765 };
});

describe("ServerPanel port shortcut", () => {
  it("offers to find what holds the port when the bind failed because it is taken", () => {
    failed("start server: listen tcp 0.0.0.0:8765: bind: address already in use");
    const html = renderToStaticMarkup(<ServerPanel onFreePort={() => undefined} />);
    expect(html).toMatch(/Find what is using port 8765/);
  });

  it("does not, for a failure a different port would not fix", () => {
    failed("accept: too many open files");
    expect(renderToStaticMarkup(<ServerPanel onFreePort={() => undefined} />)).not.toMatch(/Find what is using/);
  });

  it("does not, when the screen was given nowhere to send the person", () => {
    failed("bind: address already in use");
    expect(renderToStaticMarkup(<ServerPanel />)).not.toMatch(/Find what is using/);
  });
});
