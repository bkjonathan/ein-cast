import { describe, expect, it, vi } from "vitest";

const runtime = vi.hoisted(() => ({ EventsOn: vi.fn() }));
vi.mock("../../../wailsjs/runtime/runtime", () => runtime);

import { onEvent } from "./events";

describe("onEvent", () => {
  it("subscribes by event name and forwards payloads", () => {
    let deliver: (payload: unknown) => void = () => undefined;
    const unsubscribe = vi.fn();
    runtime.EventsOn.mockImplementation((_name: string, cb: (p: unknown) => void) => {
      deliver = cb;
      return unsubscribe;
    });

    const handler = vi.fn();
    const off = onEvent("library:scanned", handler);

    expect(runtime.EventsOn).toHaveBeenCalledWith("library:scanned", expect.any(Function));
    const payload = { root: "/m", total: 0, totalSize: 0, counts: { video: 0, audio: 0, image: 0 } };
    deliver(payload);
    expect(handler).toHaveBeenCalledWith(payload);

    off();
    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  it("decodes server:state payloads into the frontend type", () => {
    let deliver: (payload: unknown) => void = () => undefined;
    runtime.EventsOn.mockImplementation((_name: string, cb: (p: unknown) => void) => {
      deliver = cb;
      return () => undefined;
    });

    const handler = vi.fn();
    onEvent("server:state", handler);
    deliver({ state: "failed", name: "Den", port: 0, localUrl: "", lanUrls: [], error: "port in use" });

    expect(handler).toHaveBeenCalledWith({
      state: "failed",
      name: "Den",
      port: 0,
      localUrl: "",
      lanUrls: [],
      error: "port in use",
    });
  });

  it("does not deliver a server:state payload with an unknown state", () => {
    let deliver: (payload: unknown) => void = () => undefined;
    runtime.EventsOn.mockImplementation((_name: string, cb: (p: unknown) => void) => {
      deliver = cb;
      return () => undefined;
    });

    const handler = vi.fn();
    onEvent("server:state", handler);

    expect(() => deliver({ state: "paused", name: "", port: 0, localUrl: "", lanUrls: [], error: "" })).toThrow();
    expect(handler).not.toHaveBeenCalled();
  });

  it("decodes log:line payloads", () => {
    let deliver: (payload: unknown) => void = () => undefined;
    runtime.EventsOn.mockImplementation((_name: string, cb: (p: unknown) => void) => {
      deliver = cb;
      return () => undefined;
    });

    const handler = vi.fn();
    onEvent("log:line", handler);
    deliver({ seq: 3, time: 5, level: "error", source: "server", message: "boom", attrs: [] });

    expect(runtime.EventsOn).toHaveBeenLastCalledWith("log:line", expect.any(Function));
    expect(handler).toHaveBeenCalledWith({
      seq: 3, time: 5, level: "error", source: "server", message: "boom", attrs: [],
    });
  });

  it.each([
    ["library:status", { scanning: true, root: "/m", error: "", watching: false }],
    ["settings:changed", { port: 9000, name: "Den", requireToken: true, accessToken: "t", tokenPersisted: true }],
  ] as const)("delivers %s payloads", (name, payload) => {
    let deliver: (payload: unknown) => void = () => undefined;
    runtime.EventsOn.mockImplementation((_n: string, cb: (p: unknown) => void) => {
      deliver = cb;
      return () => undefined;
    });
    const handler = vi.fn();
    onEvent(name, handler);
    deliver(payload);
    expect(runtime.EventsOn).toHaveBeenLastCalledWith(name, expect.any(Function));
    expect(handler).toHaveBeenCalledWith(payload);
  });

  it("decodes remote:status payloads and rejects unknown states", () => {
    let deliver: (payload: unknown) => void = () => undefined;
    runtime.EventsOn.mockImplementation((_n: string, cb: (p: unknown) => void) => {
      deliver = cb;
      return () => undefined;
    });
    const handler = vi.fn();
    onEvent("remote:status", handler);

    const payload = {
      state: "starting", provider: "tailscale", public: false, url: "", error: "",
      live: "idle", pinSet: false, sessions: 0, needsSetup: ["ngrok"],
    };
    deliver(payload);
    expect(runtime.EventsOn).toHaveBeenLastCalledWith("remote:status", expect.any(Function));
    expect(handler).toHaveBeenCalledWith(payload);

    expect(() => deliver({ ...payload, state: "paused" })).toThrow();
    expect(() => deliver({ ...payload, live: "paused" })).toThrow();
    expect(handler).toHaveBeenCalledOnce();
  });
});
