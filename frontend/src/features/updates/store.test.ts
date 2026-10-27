import { beforeEach, describe, expect, it } from "vitest";
import type { UpdateStatus } from "@/shared/bridge";
import { useUpdatesStore } from "./store";

const status = (over: Partial<UpdateStatus> = {}): UpdateStatus => ({
  current: "1.1.1", checking: false, available: false, latest: null, checkedAt: 0, error: "", ...over,
});

beforeEach(() => useUpdatesStore.setState({ status: null }));

describe("updates store", () => {
  it("starts unknown", () => {
    expect(useUpdatesStore.getState().status).toBeNull();
  });

  it("hydrate fills an empty store", () => {
    useUpdatesStore.getState().hydrate(status());
    expect(useUpdatesStore.getState().status?.current).toBe("1.1.1");
  });

  it("an event that raced ahead of the startup query wins", () => {
    useUpdatesStore.getState().apply(status({ available: true }));
    useUpdatesStore.getState().hydrate(status({ available: false }));
    expect(useUpdatesStore.getState().status?.available).toBe(true);
  });

  it("apply always replaces", () => {
    useUpdatesStore.getState().apply(status({ checking: true }));
    useUpdatesStore.getState().apply(status({ checking: false, checkedAt: 5 }));
    expect(useUpdatesStore.getState().status).toMatchObject({ checking: false, checkedAt: 5 });
  });
});
