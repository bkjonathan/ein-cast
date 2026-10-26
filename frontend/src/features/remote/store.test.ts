import { beforeEach, describe, expect, it } from "vitest";
import type { RemoteStatus } from "@/shared/bridge";
import { useRemoteStore } from "./store";

const status = (state: RemoteStatus["state"]): RemoteStatus => ({
  state, provider: "tailscale", public: false, url: "", error: "", live: "idle", pinSet: false, sessions: 0, needsSetup: [],
});

beforeEach(() => useRemoteStore.setState({ status: null, providers: [] }));

describe("remote store", () => {
  it("starts unknown", () => {
    expect(useRemoteStore.getState().status).toBeNull();
    expect(useRemoteStore.getState().providers).toEqual([]);
  });

  it("hydrate adopts Go's current state", () => {
    useRemoteStore.getState().hydrate(status("running"));
    expect(useRemoteStore.getState().status?.state).toBe("running");
  });

  it("applyStatus always overwrites", () => {
    useRemoteStore.getState().applyStatus(status("starting"));
    useRemoteStore.getState().applyStatus(status("running"));
    expect(useRemoteStore.getState().status?.state).toBe("running");
  });

  it("an event that beats the startup query is not overwritten by it", () => {
    useRemoteStore.getState().applyStatus(status("running"));
    useRemoteStore.getState().hydrate(status("idle"));
    expect(useRemoteStore.getState().status?.state).toBe("running");
  });

  it("keeps the provider list", () => {
    useRemoteStore.getState().setProviders([{ id: "tailscale", public: false, fields: [] }]);
    expect(useRemoteStore.getState().providers).toEqual([{ id: "tailscale", public: false, fields: [] }]);
  });
});
