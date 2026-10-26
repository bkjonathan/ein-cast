import { describe, expect, it } from "vitest";
import { acknowledge, isAcknowledged, type NoticeStorage } from "./notices";

function memory(): NoticeStorage & { data: Map<string, string> } {
  const data = new Map<string, string>();
  return {
    data,
    getItem: (k) => data.get(k) ?? null,
    setItem: (k, v) => void data.set(k, v),
  };
}

describe("notice acknowledgement", () => {
  it("is not acknowledged until it is, and is remembered per provider", () => {
    const s = memory();
    expect(isAcknowledged(s, "cloudflare-quick")).toBe(false);
    acknowledge(s, "cloudflare-quick");
    expect(isAcknowledged(s, "cloudflare-quick")).toBe(true);
    expect(isAcknowledged(s, "tailscale")).toBe(false);
  });

  it("works without storage: the notice is just shown again", () => {
    expect(isAcknowledged(null, "x")).toBe(false);
    expect(() => acknowledge(null, "x")).not.toThrow();
  });

  it("survives storage that throws, as in a private window or with site data blocked", () => {
    const broken: NoticeStorage = {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("quota");
      },
    };
    expect(isAcknowledged(broken, "x")).toBe(false);
    expect(() => acknowledge(broken, "x")).not.toThrow();
  });

  it("does not take another key's value for an acknowledgement", () => {
    const s = memory();
    s.data.set("eincast.notice.cloudflare-quick", "yes"); // only "1" counts
    expect(isAcknowledged(s, "cloudflare-quick")).toBe(false);
  });
});
