import { describe, expect, it } from "vitest";
import { defaultProvider, describeProvider, setupNote } from "./providers";

const ts = { id: "tailscale", public: false, fields: [] };
const cf = { id: "cloudflare-quick", public: true, fields: [] };

describe("describeProvider", () => {
  it("knows Tailscale and recommends it", () => {
    const c = describeProvider(ts);
    expect(c.label).toBe("Tailscale");
    expect(c.recommended).toBe(true);
    expect(c.summary).toMatch(/Private/);
  });

  it("knows the Cloudflare quick tunnel: public, not recommended, with the terms notice", () => {
    const c = describeProvider(cf);
    expect(c.label).toBe("Cloudflare quick tunnel");
    expect(c.recommended).toBe(false);
    expect(c.summary).toMatch(/Public/);
    expect(c.summary).toMatch(/PIN/);
    expect(c.notice).toMatch(/terms/);
    expect(c.notice).toMatch(/Tailscale is recommended/);
  });

  it("knows the named tunnel and ngrok: public, not recommended, each with its own notice", () => {
    const named = describeProvider({ id: "cloudflare-named", public: true });
    expect(named).toMatchObject({ label: "Cloudflare named tunnel", recommended: false });
    expect(named.summary).toMatch(/PIN/);
    expect(named.notice).toMatch(/Cloudflare's service terms/);

    const ngrok = describeProvider({ id: "ngrok", public: true });
    expect(ngrok).toMatchObject({ label: "ngrok", recommended: false });
    expect(ngrok.summary).toMatch(/small data allowance/);
    expect(ngrok.notice).toMatch(/not for streaming movies/);
    expect(ngrok.notice).toMatch(/Tailscale is recommended/);
  });

  it("Tailscale has no notice to read", () => {
    expect(describeProvider(ts).notice).toBeNull();
  });

  it("describes an unfamiliar provider by its visibility, never as safe by default", () => {
    const unknown = { id: "new-thing", public: true };
    expect(describeProvider(unknown)).toMatchObject({ label: "new-thing", recommended: false });
    expect(describeProvider(unknown).summary).toMatch(/Public/);
    expect(describeProvider(unknown).summary).toMatch(/PIN/);
    expect(describeProvider({ id: "x", public: false }).summary).toMatch(/Private/);
    expect(describeProvider({ id: "x", public: true }).notice).toBeNull();
  });
});

describe("defaultProvider", () => {
  it("prefers the one in use, then the user's pick, then the recommended one", () => {
    expect(defaultProvider([cf, ts], "cloudflare-quick", "tailscale")).toBe("cloudflare-quick");
    expect(defaultProvider([cf, ts], "", "cloudflare-quick")).toBe("cloudflare-quick");
    expect(defaultProvider([cf, ts], "", null)).toBe("tailscale");
  });

  it("ignores ids that are not offered", () => {
    expect(defaultProvider([cf], "gone", "also-gone")).toBe("cloudflare-quick");
  });

  it("is empty when nothing is offered", () => {
    expect(defaultProvider([], "", null)).toBe("");
  });
});

describe("setupNote", () => {
  it("tells the person which address to give the Cloudflare tunnel, using the server's port", () => {
    expect(setupNote("cloudflare-named", 8765)).toMatch(/http:\/\/localhost:8765/);
  });

  it("still says something useful before the port is known", () => {
    expect(setupNote("cloudflare-named", undefined)).toMatch(/port shown in Settings/);
  });

  it("has nothing to add for the others", () => {
    for (const id of ["tailscale", "cloudflare-quick", "ngrok", "unknown"]) expect(setupNote(id, 8765)).toBeNull();
  });
});
