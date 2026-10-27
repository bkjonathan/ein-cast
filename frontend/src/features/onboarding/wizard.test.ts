import { describe, expect, it } from "vitest";
import type { LogLine } from "@/shared/bridge";
import { translate } from "@/shared/i18n";
import { firewallNote, firstDevice, stepNumber, wizardSteps } from "./wizard";

const request = (seq: number, attrs: Record<string, string>, over: Partial<LogLine> = {}): LogLine => ({
  seq,
  time: 0,
  level: "info",
  source: "http",
  message: "request",
  attrs: Object.entries(attrs).map(([key, value]) => ({ key, value })),
  ...over,
});

describe("firstDevice", () => {
  it("finds the first answered request from another address", () => {
    const lines = [
      request(1, { client_ip: "127.0.0.1", status: "200", path: "/" }),
      request(2, { client_ip: "192.168.1.20", status: "200", path: "/" }),
      request(3, { client_ip: "192.168.1.21", status: "200", path: "/api/library" }),
    ];
    expect(firstDevice(lines, 0)).toStrictEqual({ client: "192.168.1.20" });
  });

  it("ignores this computer, in either spelling of loopback", () => {
    const lines = [
      request(1, { client_ip: "127.0.0.1", status: "200", path: "/" }),
      request(2, { client_ip: "::1", status: "200", path: "/" }),
    ];
    expect(firstDevice(lines, 0)).toBeNull();
  });

  it("ignores requests that were turned away, so a device without the token does not count as connected", () => {
    expect(firstDevice([request(1, { client_ip: "192.168.1.20", status: "401", path: "/" })], 0)).toBeNull();
    expect(firstDevice([request(1, { client_ip: "192.168.1.20", status: "404", path: "/x" })], 0)).toBeNull();
  });

  it("ignores the browser's icon request and lines that are not access-log lines", () => {
    const lines = [
      request(1, { client_ip: "192.168.1.20", status: "200", path: "/favicon.ico" }),
      request(2, { client_ip: "192.168.1.20", status: "200", path: "/" }, { source: "library" }),
      request(3, { client_ip: "192.168.1.20", status: "200", path: "/" }, { message: "scan finished" }),
    ];
    expect(firstDevice(lines, 0)).toBeNull();
  });

  it("only counts requests after the wizard opened", () => {
    const lines = [request(5, { client_ip: "192.168.1.20", status: "200", path: "/" })];
    expect(firstDevice(lines, 5)).toBeNull();
    expect(firstDevice(lines, 4)).toStrictEqual({ client: "192.168.1.20" });
  });

  it("treats a request with no usable status or address as nothing", () => {
    expect(firstDevice([request(1, { client_ip: "192.168.1.20", path: "/" })], 0)).toBeNull();
    expect(firstDevice([request(1, { status: "200", path: "/" })], 0)).toBeNull();
  });
});

describe("steps", () => {
  it("numbers the steps from 1 in order", () => {
    expect(wizardSteps.map((s) => stepNumber(s.id))).toStrictEqual([1, 2, 3]);
  });
});

describe("firewallNote", () => {
  it("tells Windows users which box to tick, because the default leaves private Wi-Fi blocked", () => {
    expect(translate("en", firewallNote("windows"))).toMatch(/Private networks/);
    expect(translate("en", firewallNote("darwin"))).toMatch(/Allow/);
    expect(translate("en", firewallNote(""))).toMatch(/allow/i);
  });
});
