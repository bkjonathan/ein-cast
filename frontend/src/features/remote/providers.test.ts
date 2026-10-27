import { describe, expect, it } from "vitest";
import { translate, translatorFor, type MessageKey } from "@/shared/i18n";
import { defaultProvider, describeProvider, setupNote, type ProviderCopy } from "./providers";

const en = translatorFor("en");
const summaryOf = (c: ProviderCopy) => translate("en", c.summaryKey);
const noticeOf = (c: ProviderCopy) => (c.noticeKey === null ? null : translate("en", c.noticeKey as MessageKey));

const ts = { id: "tailscale", public: false, fields: [] };
const cf = { id: "cloudflare-quick", public: true, fields: [] };

describe("describeProvider", () => {
  it("knows Tailscale and recommends it", () => {
    const c = describeProvider(ts);
    expect(c.label).toBe("Tailscale");
    expect(c.recommended).toBe(true);
    expect(summaryOf(c)).toMatch(/Private/);
  });

  it("knows the Cloudflare quick tunnel: public, not recommended, with the terms notice", () => {
    const c = describeProvider(cf);
    expect(c.label).toBe("Cloudflare quick tunnel");
    expect(c.recommended).toBe(false);
    expect(summaryOf(c)).toMatch(/Public/);
    expect(summaryOf(c)).toMatch(/PIN/);
    expect(noticeOf(c)).toMatch(/terms/);
    expect(noticeOf(c)).toMatch(/Tailscale is recommended/);
  });

  it("knows the named tunnel and ngrok: public, not recommended, each with its own notice", () => {
    const named = describeProvider({ id: "cloudflare-named", public: true });
    expect(named).toMatchObject({ label: "Cloudflare named tunnel", recommended: false });
    expect(summaryOf(named)).toMatch(/PIN/);
    expect(noticeOf(named)).toMatch(/Cloudflare's service terms/);

    const ngrok = describeProvider({ id: "ngrok", public: true });
    expect(ngrok).toMatchObject({ label: "ngrok", recommended: false });
    expect(summaryOf(ngrok)).toMatch(/small data allowance/);
    expect(noticeOf(ngrok)).toMatch(/not for streaming movies/);
    expect(noticeOf(ngrok)).toMatch(/Tailscale is recommended/);
  });

  it("Tailscale has no notice to read", () => {
    expect(noticeOf(describeProvider(ts))).toBeNull();
  });

  it("describes an unfamiliar provider by its visibility, never as safe by default", () => {
    const unknown = { id: "new-thing", public: true };
    expect(describeProvider(unknown)).toMatchObject({ label: "new-thing", recommended: false });
    expect(summaryOf(describeProvider(unknown))).toMatch(/Public/);
    expect(summaryOf(describeProvider(unknown))).toMatch(/PIN/);
    expect(summaryOf(describeProvider({ id: "x", public: false }))).toMatch(/Private/);
    expect(noticeOf(describeProvider({ id: "x", public: true }))).toBeNull();
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
    expect(setupNote("cloudflare-named", 8765, en)).toMatch(/http:\/\/localhost:8765/);
  });

  it("still says something useful before the port is known", () => {
    expect(setupNote("cloudflare-named", undefined, en)).toMatch(/port shown in Settings/);
  });

  it("has nothing to add for the others", () => {
    for (const id of ["tailscale", "cloudflare-quick", "ngrok", "unknown"]) expect(setupNote(id, 8765, en)).toBeNull();
  });
});
