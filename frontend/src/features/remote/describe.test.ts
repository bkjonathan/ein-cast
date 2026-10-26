import { describe, expect, it } from "vitest";
import type { RemoteProvider, RemoteStatus } from "@/shared/bridge";
import { enableBlocker, publicBadge, remoteActions, summarize } from "./describe";

const status = (over: Partial<RemoteStatus> = {}): RemoteStatus => ({
  state: "idle", provider: "", public: false, url: "", error: "", live: "idle",
  pinSet: false, sessions: 0, needsSetup: [], ...over,
});

describe("summarize", () => {
  it("labels every state", () => {
    expect(summarize(status({ state: "idle" }))).toStrictEqual({ label: "Off", tone: "neutral" });
    expect(summarize(status({ state: "starting" }))).toStrictEqual({ label: "Connecting…", tone: "warning" });
    expect(summarize(status({ state: "stopping" }))).toStrictEqual({ label: "Turning off…", tone: "warning" });
    expect(summarize(status({ state: "failed" }))).toStrictEqual({ label: "Failed", tone: "danger" });
  });

  it("running is green when private and amber when public", () => {
    const base = { state: "running", live: "running" } as const;
    expect(summarize(status({ ...base, public: false })).tone).toBe("success");
    expect(summarize(status({ ...base, public: true })).tone).toBe("warning");
  });

  it("shows reconnecting while the provider has lost its connection", () => {
    expect(summarize(status({ state: "running", live: "starting" }))).toStrictEqual({
      label: "Reconnecting…", tone: "warning",
    });
  });
});

describe("remoteActions", () => {
  const cases: [string, Partial<RemoteStatus>, boolean, boolean][] = [
    ["off", { state: "idle" }, true, false],
    ["connecting: can only cancel", { state: "starting", provider: "tailscale" }, false, true],
    ["on", { state: "running", provider: "tailscale" }, false, true],
    ["turning off", { state: "stopping", provider: "tailscale" }, false, false],
    ["never came up: retry", { state: "failed", provider: "" }, true, false],
    ["could not confirm it stopped: turn off again", { state: "failed", provider: "tailscale", public: true }, false, true],
  ];
  it.each(cases)("%s", (_name, over, canEnable, canDisable) => {
    expect(remoteActions(status(over))).toStrictEqual({ canEnable, canDisable });
  });
});

describe("enableBlocker", () => {
  const priv: RemoteProvider = { id: "tailscale", public: false, fields: [] };
  const pub: RemoteProvider = { id: "cloudflare-quick", public: true, fields: [] };

  it("needs a running server first", () => {
    expect(enableBlocker(priv, status(), false)).toMatch(/server/);
  });
  it("needs a choice", () => {
    expect(enableBlocker(undefined, status(), true)).toMatch(/Choose/);
  });
  it("a provider that needs setting up is held back first, before the PIN and the notice", () => {
    const named: RemoteProvider = { id: "cloudflare-named", public: true, fields: [] };
    const needs = status({ needsSetup: ["cloudflare-named"], pinSet: false });
    expect(enableBlocker(named, needs, true, true)).toMatch(/setting this up/);
    // Only the provider that needs it is held back.
    expect(enableBlocker(named, status({ needsSetup: ["ngrok"], pinSet: true }), true)).toBeNull();
    // And the server still comes first.
    expect(enableBlocker(named, needs, false)).toMatch(/server/);
  });

  it("holds back a provider whose notice has not been read, after the PIN rule", () => {
    expect(enableBlocker(pub, status({ pinSet: true }), true, true)).toMatch(/notice/);
    expect(enableBlocker(pub, status({ pinSet: true }), true, false)).toBeNull();
    // The PIN is the more fundamental problem, so it is reported first.
    expect(enableBlocker(pub, status({ pinSet: false }), true, true)).toMatch(/PIN/);
  });
  it("a public provider needs a PIN; a private one does not", () => {
    expect(enableBlocker(pub, status({ pinSet: false }), true)).toMatch(/PIN/);
    expect(enableBlocker(pub, status({ pinSet: true }), true)).toBeNull();
    expect(enableBlocker(priv, status({ pinSet: false }), true)).toBeNull();
  });
});

describe("publicBadge", () => {
  it("is absent unless the library may be public", () => {
    expect(publicBadge(null)).toBeNull();
    expect(publicBadge(status({ state: "running", public: false }))).toBeNull();
    expect(publicBadge(status())).toBeNull();
  });

  it("says ON, with the number signed in", () => {
    const on = { state: "running", public: true } as const;
    expect(publicBadge(status({ ...on, sessions: 0 }))).toBe("Public access ON");
    expect(publicBadge(status({ ...on, sessions: 3 }))).toBe("Public access ON · 3 signed in");
  });

  it("stays on while it connects, because it may already be reachable", () => {
    expect(publicBadge(status({ state: "starting", public: true }))).toBe("Public access ON");
  });

  it("does not claim it is off until Go confirms it", () => {
    expect(publicBadge(status({ state: "stopping", public: true }))).toMatch(/turning off/);
    expect(publicBadge(status({ state: "failed", provider: "x", public: true }))).toMatch(/may still be ON/);
  });
});
