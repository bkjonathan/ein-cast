import { describe, expect, it } from "vitest";
import type { RemoteStatus, ServerStatus } from "@/shared/bridge";
import { connectSources } from "./sources";

const server = (lanUrls: string[]): ServerStatus => ({
  state: "running", name: "Den", port: 8765, localUrl: "http://127.0.0.1:8765", lanUrls, nameUrls: [], discovery: [], error: "",
});

const remote = (over: Partial<RemoteStatus> = {}): RemoteStatus => ({
  state: "running", provider: "tailscale", public: false, url: "http://den.ts.net:8765", error: "",
  live: "running", pinSet: false, sessions: 0, needsSetup: [], ...over,
});

describe("connectSources", () => {
  it("lists each LAN address with the access token", () => {
    expect(connectSources(server(["http://192.168.1.20:8765", "http://10.0.0.4:8765"]), null, "tok", "Tailscale")).toStrictEqual([
      { kind: "lan", label: "192.168.1.20", base: "http://192.168.1.20:8765", token: "tok", playlist: true },
      { kind: "lan", label: "10.0.0.4", base: "http://10.0.0.4:8765", token: "tok", playlist: true },
    ]);
  });

  it("offers the announced name after the numeric addresses, with the same token rules", () => {
    const status: ServerStatus = { ...server(["http://192.168.1.20:8765"]), nameUrls: ["http://eincast.local:8765"], discovery: ["mdns"] };
    expect(connectSources(status, null, "tok", "Tailscale")).toStrictEqual([
      { kind: "lan", label: "192.168.1.20", base: "http://192.168.1.20:8765", token: "tok", playlist: true },
      { kind: "lan", label: "eincast.local", base: "http://eincast.local:8765", token: "tok", playlist: true },
    ]);
  });

  it("adds a private tunnel after the LAN addresses; it follows the LAN rules, token included", () => {
    const got = connectSources(server(["http://192.168.1.20:8765"]), remote(), "tok", "Tailscale");
    expect(got.map((s) => s.kind)).toEqual(["lan", "private"]);
    expect(got[1]).toStrictEqual({
      kind: "private", label: "Tailscale", base: "http://den.ts.net:8765", token: "tok", playlist: true,
    });
  });

  it("a public tunnel never carries the LAN token, and offers no bare playlist", () => {
    const got = connectSources(server([]), remote({ public: true, url: "https://abc.trycloudflare.com" }), "tok", "Cloudflare");
    expect(got).toStrictEqual([
      { kind: "public", label: "Cloudflare", base: "https://abc.trycloudflare.com", token: "", playlist: false },
    ]);
  });

  it("offers the tunnel even when there is no LAN", () => {
    expect(connectSources(server([]), remote(), "", "Tailscale")).toHaveLength(1);
  });

  it.each([
    ["off", { state: "idle", url: "" }],
    ["connecting", { state: "starting", url: "" }],
    ["turning off", { state: "stopping" }],
    ["failed", { state: "failed" }],
    ["reconnecting: the address would not work", { live: "starting" }],
    ["no address yet", { url: "" }],
  ] as const)("does not offer a tunnel that is %s", (_name, over) => {
    expect(connectSources(server([]), remote(over), "", "Tailscale")).toEqual([]);
  });

  it("copes with remote access not being known yet", () => {
    expect(connectSources(server(["http://192.168.1.20:8765"]), null, "", "x")).toHaveLength(1);
  });
});
