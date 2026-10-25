import { beforeEach, describe, expect, it } from "vitest";
import type { ServerStatus } from "@/shared/bridge";
import { useServerStore } from "./store";

const status = (state: ServerStatus["state"]): ServerStatus => ({
  state,
  name: "Den",
  addr: "",
  error: "",
});

beforeEach(() => useServerStore.setState({ status: null }));

describe("server store", () => {
  it("starts unknown", () => {
    expect(useServerStore.getState().status).toBeNull();
  });

  it("hydrate adopts Go's current state", () => {
    useServerStore.getState().hydrate(status("running"));
    expect(useServerStore.getState().status?.state).toBe("running");
  });

  it("applyStatus always overwrites", () => {
    useServerStore.getState().applyStatus(status("starting"));
    useServerStore.getState().applyStatus(status("running"));
    expect(useServerStore.getState().status?.state).toBe("running");
  });

  it("an event that beats the startup query is not overwritten by it", () => {
    useServerStore.getState().applyStatus(status("running"));
    useServerStore.getState().hydrate(status("idle"));
    expect(useServerStore.getState().status?.state).toBe("running");
  });
});
