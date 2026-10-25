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
});
