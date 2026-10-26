import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RemoteProvider, RemoteStatus, ServerStatus } from "@/shared/bridge";
import type { RemoteFieldState } from "@/shared/bridge";
import { PinPanel } from "./PinPanel";
import { ProviderPanel } from "./ProviderPanel";
import { PublicAccessBadge } from "./PublicAccessBadge";

// Server rendering reads a zustand store's *initial* state, so setState from a
// test would be invisible. These tests are about what the components draw for a
// given state; the stores themselves are tested in store.test.ts. So the store
// hooks are replaced by plain selectors over state the test controls.
const mem = vi.hoisted(() => ({
  remote: { status: null, providers: [] } as { status: RemoteStatus | null; providers: RemoteProvider[] },
  server: { status: null } as { status: ServerStatus | null },
  settings: { settings: { port: 8765 } } as { settings: { port: number } | null },
  // What the settings form is told Go has saved. Null: not loaded yet.
  config: { states: null } as { states: RemoteFieldState[] | null },
}));
vi.mock("@/features/settings", () => ({
  useSettingsStore: <T,>(select: (s: typeof mem.settings) => T): T => select(mem.settings),
}));
vi.mock("../hooks/useProviderConfig", () => ({
  useProviderConfig: () => ({
    states: mem.config.states,
    save: async () => true,
    remove: async () => true,
    browse: async () => "",
    busyKey: null,
    error: null,
  }),
}));
vi.mock("../store", () => ({
  useRemoteStore: <T,>(select: (s: typeof mem.remote) => T): T => select(mem.remote),
}));
vi.mock("@/features/server", () => ({
  useServerStore: <T,>(select: (s: typeof mem.server) => T): T => select(mem.server),
}));

const tailscale: RemoteProvider = { id: "tailscale", public: false, fields: [] };
const cloudflare: RemoteProvider = { id: "cloudflare-quick", public: true, fields: [] };

const remote = (over: Partial<RemoteStatus> = {}): RemoteStatus => ({
  state: "idle", provider: "", public: false, url: "", error: "", live: "idle",
  pinSet: false, sessions: 0, needsSetup: [], ...over,
});

const server = (state: ServerStatus["state"]): ServerStatus => ({
  state, name: "Den", port: 8765, localUrl: "", lanUrls: [], error: "",
});

function show(status: RemoteStatus | null, providers: RemoteProvider[], serverState: ServerStatus["state"] = "running") {
  mem.remote = { status, providers };
  mem.server = { status: server(serverState) };
}

// Find a rendered <button> by its text.
function button(html: string, text: string): string | undefined {
  return [...html.matchAll(/<button\b[^>]*>.*?<\/button>/gs)].map((m) => m[0]).find((b) => b.includes(text));
}

// The disabled *attribute*. Every button's class list contains "disabled:…"
// utilities, so a plain /disabled/ would match them all.
function isDisabled(buttonHtml: string | undefined): boolean {
  if (buttonHtml === undefined) throw new Error("no such button");
  return /\sdisabled(?:=""|\s|>)/.test(buttonHtml.slice(0, buttonHtml.indexOf(">")));
}

beforeEach(() => {
  mem.remote = { status: null, providers: [] };
  mem.server = { status: null };
  mem.settings = { settings: { port: 8765 } };
  mem.config = { states: null };
});

describe("PublicAccessBadge", () => {
  const render = () => renderToStaticMarkup(<PublicAccessBadge onOpen={() => undefined} />);

  it("renders nothing before Go reports, and nothing for a private tunnel", () => {
    expect(render()).toBe("");
    show(remote({ state: "running", live: "running", provider: "tailscale", public: false }), [tailscale]);
    expect(render()).toBe("");
  });

  it("warns while a public tunnel is on, with the number signed in", () => {
    show(remote({ state: "running", live: "running", provider: "cloudflare-quick", public: true, sessions: 2 }), [cloudflare]);
    expect(render()).toContain("Public access ON · 2 signed in");
  });

  it("does not claim to be off while a stop is unconfirmed", () => {
    show(remote({ state: "failed", provider: "cloudflare-quick", public: true }), [cloudflare]);
    expect(render()).toContain("may still be ON");
  });
});

describe("ProviderPanel", () => {
  const render = () => renderToStaticMarkup(<ProviderPanel />);

  it("renders nothing until Go reports", () => {
    expect(render()).toBe("");
  });

  it("offers Tailscale as the recommended, private choice, and can turn it on", () => {
    show(remote(), [tailscale]);
    const html = render();
    expect(html).toContain("Tailscale");
    expect(html).toContain("Recommended");
    expect(html).toContain("Private");
    expect(isDisabled(button(html, "Turn on"))).toBe(false);
  });

  it("says to start the server first, and will not turn on without one", () => {
    show(remote(), [tailscale], "idle");
    const html = render();
    expect(html).toContain("Start the server first.");
    expect(isDisabled(button(html, "Turn on"))).toBe(true);
  });

  it("a public provider needs a PIN and shows the warning", () => {
    show(remote({ pinSet: false }), [cloudflare]);
    const html = render();
    expect(html).toContain("Set an access PIN first.");
    expect(html).toContain("Only your PIN");
    expect(isDisabled(button(html, "Turn on"))).toBe(true);

    // With the PIN set, the next thing in the way is the notice (tested below).
    show(remote({ pinSet: true }), [cloudflare]);
    const html2 = render();
    expect(html2).not.toContain("Set an access PIN first.");
    expect(html2).toContain("Read the notice above first.");
  });

  it("holds Turn on back until the Cloudflare terms notice is read", () => {
    show(remote({ pinSet: true }), [cloudflare]);
    const html = render();
    expect(html).toContain("service terms");
    expect(html).toContain("Tailscale is recommended");
    expect(button(html, "I understand")).toBeDefined();
    expect(html).toContain("Read the notice above first.");
    expect(isDisabled(button(html, "Turn on"))).toBe(true);
  });

  it("shows no notice for Tailscale", () => {
    show(remote(), [tailscale]);
    const html = render();
    expect(html).not.toContain("I understand");
    expect(html).not.toContain("service terms");
  });

  it("always mentions the upload-speed limit", () => {
    show(remote(), [tailscale]);
    expect(render()).toContain("upload speed");
  });

  it("shows the address and Turn off while running", () => {
    show(remote({ state: "running", live: "running", provider: "tailscale", url: "http://den.ts.net:8765" }), [tailscale]);
    const html = render();
    expect(html).toContain("http://den.ts.net:8765");
    expect(button(html, "Turn off")).toBeDefined();
    expect(button(html, "Turn on")).toBeUndefined();
    // The choice is locked while it is on.
    expect(html).toMatch(/role="radio"[^>]*\sdisabled(?:=""|\s|>)/);
  });

  it("a slow start can be cancelled", () => {
    show(remote({ state: "starting", provider: "tailscale" }), [tailscale]);
    expect(isDisabled(button(render(), "Cancel"))).toBe(false);
  });

  it("a start that failed shows why and offers a retry; an unconfirmed stop offers to turn off again", () => {
    show(remote({ state: "failed", error: "Tailscale is turned off. Turn it on in the Tailscale app, then try again" }), [tailscale]);
    let html = render();
    expect(html).toContain("Tailscale is turned off");
    expect(button(html, "Try again")).toBeDefined();

    show(remote({ state: "failed", provider: "tailscale", error: "process will not die" }), [tailscale]);
    html = render();
    expect(button(html, "Try turning off again")).toBeDefined();
    expect(button(html, "Try again")).toBeUndefined();
  });

  it("says so when the build offers no options", () => {
    show(remote(), []);
    expect(render()).toContain("No remote access options");
  });

  it("escapes anything Go sends that ends up in the page", () => {
    show(remote({ state: "failed", error: "<img src=x onerror=alert(1)>" }), [tailscale]);
    const html = render();
    expect(html).not.toContain("<img");
    expect(html).toContain("&lt;img");
  });
});

// A public provider that needs a token and a host name, and optionally a program.
const named: RemoteProvider = {
  id: "cloudflare-named",
  public: true,
  fields: [
    { key: "token", label: "Tunnel token", help: "Paste the token.", placeholder: "eyJ…", kind: "secret", required: true },
    { key: "hostname", label: "Hostname", help: "The public hostname.", placeholder: "media.example.com", kind: "text", required: true },
    { key: "binary", label: "cloudflared location", help: "", placeholder: "", kind: "file", required: false },
  ],
};

describe("ProviderPanel setup", () => {
  const render = () => renderToStaticMarkup(<ProviderPanel />);

  it("asks for what is missing and holds Turn on back", () => {
    show(remote({ needsSetup: ["cloudflare-named"], pinSet: true }), [named]);
    const html = render();

    expect(html).toContain("Needs setup");
    expect(html).toContain("Set up Cloudflare named tunnel");
    expect(html).toContain("Tunnel token");
    expect(html).toContain("Required");
    expect(html).toMatch(/type="password"[^>]*placeholder="eyJ…"|placeholder="eyJ…"[^>]*type="password"/);
    expect(html).toContain("Finish setting this up first.");
    expect(isDisabled(button(html, "Turn on"))).toBe(true);
  });

  it("tells the person which port to give Cloudflare", () => {
    show(remote({ needsSetup: ["cloudflare-named"] }), [named]);
    expect(render()).toContain("http://localhost:8765");
  });

  it("shows a saved secret as saved, with no way to read it back", () => {
    show(remote({ needsSetup: [] }), [named]);
    mem.config = {
      states: [
        // Even if a value reached the UI by mistake, it must not be drawn.
        { key: "token", set: true, value: "LEAKED-SECRET-VALUE" },
        { key: "hostname", set: true, value: "media.example.com" },
        { key: "binary", set: false, value: "" },
      ],
    };
    const html = render();

    expect(html).toContain("Saved in your keychain");
    expect(button(html, "Replace")).toBeDefined();
    expect(html).not.toContain("LEAKED-SECRET-VALUE");
    expect(html).not.toMatch(/placeholder="eyJ…"/); // no empty box asking for it again
    // The host name is not a secret, so it is shown for editing.
    expect(html).toContain('value="media.example.com"');
    expect(html).not.toContain("Needs setup");
  });

  it("never puts a secret's value into the input, whatever state it arrives in", () => {
    show(remote({ needsSetup: ["cloudflare-named"] }), [named]);
    // Not set, yet carrying a value: an upstream bug. The form must not echo it.
    mem.config = { states: [{ key: "token", set: false, value: "LEAKED-IN-A-BOX" }] };
    const html = render();

    expect(html).toMatch(/type="password"/);
    expect(html).not.toContain("LEAKED-IN-A-BOX");
  });

  it("offers a file picker for a file setting", () => {
    show(remote({ needsSetup: ["cloudflare-named"] }), [named]);
    expect(button(render(), "Browse")).toBeDefined();
  });

  it("freezes the settings while the provider is on, and says why", () => {
    show(remote({ state: "running", live: "running", provider: "cloudflare-named", url: "https://media.example.com", public: true, pinSet: true }), [named]);
    mem.config = { states: [{ key: "token", set: false, value: "" }, { key: "hostname", set: true, value: "media.example.com" }, { key: "binary", set: false, value: "" }] };
    const html = render();

    expect(html).toContain("off to change its settings");
    for (const input of html.match(/<input\b[^>]*>/g) ?? []) {
      if (input.includes('type="radio"')) continue;
      expect(input).toMatch(/\sdisabled(?:=""|\s|>)/);
    }
  });

  it("keeps a provider with only optional settings tucked away", () => {
    const quick: RemoteProvider = {
      id: "cloudflare-quick",
      public: true,
      fields: [{ key: "binary", label: "cloudflared location", help: "", placeholder: "", kind: "file", required: false }],
    };
    show(remote({ pinSet: true }), [quick]);
    const html = render();

    expect(html).toContain("<details");
    expect(html).toContain("Advanced settings");
    expect(html).not.toContain("Set up Cloudflare");
    expect(html).not.toContain("Finish setting this up first.");
  });

  it("draws nothing extra for a provider that needs no settings", () => {
    show(remote(), [tailscale]);
    const html = render();
    expect(html).not.toContain("Advanced settings");
    expect(html).not.toContain("Set up");
  });

  it("escapes help text and values it is given", () => {
    const hostile: RemoteProvider = {
      id: "cloudflare-named",
      public: true,
      fields: [{ key: "hostname", label: "Host<script>", help: "<img src=x onerror=alert(1)>", placeholder: "", kind: "text", required: true }],
    };
    show(remote({ needsSetup: ["cloudflare-named"] }), [hostile]);
    mem.config = { states: [{ key: "hostname", set: true, value: '"><script>alert(1)</script>' }] };
    const html = render();

    expect(html).not.toContain("<script>");
    expect(html).not.toContain("<img src=x");
  });
});

describe("PinPanel", () => {
  const render = () => renderToStaticMarkup(<PinPanel />);

  it("is absent when no public provider is offered and no PIN exists", () => {
    show(remote(), [tailscale]);
    expect(render()).toBe("");
  });

  it("asks for a PIN when a public provider is offered", () => {
    show(remote(), [tailscale, cloudflare]);
    const html = render();
    expect(html).toContain('type="password"');
    expect(html).toContain("Set PIN");
    expect(html).toContain("At least 8 characters");
  });

  it("with a PIN set, offers change and remove", () => {
    show(remote({ pinSet: true }), [cloudflare]);
    const html = render();
    expect(html).toContain("A PIN is set.");
    expect(button(html, "Change PIN")).toBeDefined();
    expect(isDisabled(button(html, "Remove PIN"))).toBe(false);
    expect(html).not.toContain('type="password"');
  });

  it("will not remove the PIN while public access is on", () => {
    show(remote({ pinSet: true, state: "running", live: "running", provider: "cloudflare-quick", public: true }), [cloudflare]);
    const html = render();
    expect(isDisabled(button(html, "Remove PIN"))).toBe(true);
    expect(html).toContain("Turn off public access to remove it.");
  });
});
