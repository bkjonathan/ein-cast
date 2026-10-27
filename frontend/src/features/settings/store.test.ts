import { beforeEach, describe, expect, it } from "vitest";
import type { Settings } from "@/shared/bridge";
import { useSettingsStore } from "./store";

const settings = (over: Partial<Settings> = {}): Settings => ({
  port: 8765,
  name: "EinCast",
  requireToken: false,
  dlna: false,
  autoStart: false,
  keepAwake: true,
  openAtLogin: false,
  hideOnClose: false,
  ffmpegPath: "",
  hardwareEncoding: true,
  accessToken: "",
  tokenPersisted: false,
  ...over,
});

beforeEach(() => useSettingsStore.setState({ settings: null }));

describe("settings store", () => {
  it("starts unknown", () => {
    expect(useSettingsStore.getState().settings).toBeNull();
  });

  it("hydrate adopts Go's settings", () => {
    useSettingsStore.getState().hydrate(settings({ port: 9000 }));
    expect(useSettingsStore.getState().settings?.port).toBe(9000);
  });

  it("apply always overwrites", () => {
    useSettingsStore.getState().apply(settings({ port: 9000 }));
    useSettingsStore.getState().apply(settings({ port: 9100 }));
    expect(useSettingsStore.getState().settings?.port).toBe(9100);
  });

  it("a settings event that beats the startup query is not overwritten by it", () => {
    useSettingsStore.getState().apply(settings({ requireToken: true, accessToken: "new" }));
    useSettingsStore.getState().hydrate(settings({ requireToken: false }));
    expect(useSettingsStore.getState().settings).toMatchObject({ requireToken: true, accessToken: "new" });
  });
});
