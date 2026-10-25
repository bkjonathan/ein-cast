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
});
