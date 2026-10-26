import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RemoteProvider, RemoteStatus, ServerStatus, Settings } from "@/shared/bridge";
import { ConnectCard } from "./ConnectCard";

// See components.test.tsx in features/remote: server rendering cannot see a
// zustand store's updated state, so the store hooks are replaced by plain
// selectors over state the test controls.
const mem = vi.hoisted(() => ({
  server: { status: null } as { status: ServerStatus | null },
  remote: { status: null, providers: [] } as { status: RemoteStatus | null; providers: RemoteProvider[] },
  settings: { settings: null } as { settings: Settings | null },
}));
vi.mock("@/features/server", () => ({
  useServerStore: <T,>(select: (s: typeof mem.server) => T): T => select(mem.server),
}));
vi.mock("@/features/settings", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
}));
vi.mock("@/features/remote", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/remote")>()),
  useRemoteStore: <T,>(select: (s: typeof mem.remote) => T): T => select(mem.remote),
}));

const server = (over: Partial<ServerStatus> = {}): ServerStatus => ({
  state: "running", name: "Den", port: 8765, localUrl: "http://127.0.0.1:8765",
  lanUrls: ["http://192.168.1.20:8765"], error: "", ...over,
});

const remote = (over: Partial<RemoteStatus> = {}): RemoteStatus => ({
  state: "running", provider: "tailscale", public: false, url: "http://den.ts.net:8765", error: "",
  live: "running", pinSet: false, sessions: 0, needsSetup: [], ...over,
});

const render = () => renderToStaticMarkup(<ConnectCard />);

const setServer = (status: ServerStatus) => (mem.server = { status });
const setRemote = (status: RemoteStatus | null) => (mem.remote = { status, providers: [] });
const setSettings = (settings: Settings | null) => (mem.settings = { settings });

beforeEach(() => {
  setServer(server());
  setRemote(null);
  setSettings(null);
});

describe("ConnectCard", () => {
  it("shows nothing unless the server is running", () => {
    setServer(server({ state: "idle" }));
    expect(render()).toBe("");
  });

  it("with only a LAN, behaves as before: one address, a playlist, no tabs", () => {
    const html = render();
    expect(html).toContain("http://192.168.1.20:8765/");
    expect(html).toContain("Playlist for VLC / Infuse");
    expect(html).not.toContain("Where from");
  });

  it("adds a tab for a private tunnel and keeps the LAN selected", () => {
    setRemote(remote());
    const html = render();
    expect(html).toContain("Where from");
    expect(html).toContain("Tailscale");
    expect(html).toContain("192.168.1.20");
    expect(html).toContain("same Wi-Fi");
  });

  it("with no LAN, a private tunnel is shown directly, with its playlist", () => {
    setServer(server({ lanUrls: [] }));
    setRemote(remote());
    const html = render();
    expect(html).not.toContain("No local network found");
    expect(html).toContain("http://den.ts.net:8765/");
    expect(html).toContain("Playlist for VLC / Infuse");
  });

  it("a public tunnel shows the PIN advice and no bare playlist link, and never the LAN token", () => {
    setServer(server({ lanUrls: [] }));
    setSettings({ port: 8765, name: "Den", requireToken: true, accessToken: "SECRET-LAN-TOKEN", tokenPersisted: true });
    setRemote(remote({ public: true, provider: "cloudflare-quick", url: "https://abc.trycloudflare.com" }));
    const html = render();

    expect(html).toContain(">https://abc.trycloudflare.com<"); // shown exactly, with no token appended
    expect(html).toContain("enter your PIN");
    expect(html).toContain("sign in on the player page first");
    expect(html).not.toContain("playlist.m3u");
    expect(html).not.toContain("SECRET-LAN-TOKEN");
    expect(html).not.toContain("include your access token");
  });

  it("the LAN token is still added to LAN links", () => {
    setSettings({ port: 8765, name: "Den", requireToken: true, accessToken: "SECRET-LAN-TOKEN", tokenPersisted: true });
    expect(render()).toContain("t=SECRET-LAN-TOKEN");
  });

  it("explains when there is no network at all", () => {
    setServer(server({ lanUrls: [] }));
    expect(render()).toContain("No local network found");
  });

  it("does not offer a tunnel that has not come up", () => {
    setRemote(remote({ state: "starting", url: "" }));
    expect(render()).not.toContain("Where from");
  });
});
