import { beforeEach, describe, expect, it, vi } from "vitest";

const go = vi.hoisted(() => ({
  AppInfo: vi.fn(),
  ChooseFolder: vi.fn(),
  GetLibrarySummary: vi.fn(),
}));
vi.mock("../../../wailsjs/go/bridge/App", () => go);

import { chooseFolder, getAppInfo, getLibrarySummary } from "./commands";

beforeEach(() => vi.resetAllMocks());

describe("getAppInfo", () => {
  it("maps the generated DTO into a plain object", async () => {
    go.AppInfo.mockResolvedValue({ name: "EinCast", version: "1", platform: "darwin", extra: "x" });
    await expect(getAppInfo()).resolves.toStrictEqual({
      name: "EinCast",
      version: "1",
      platform: "darwin",
    });
  });
});

describe("getLibrarySummary", () => {
  it("returns null when Go has no library yet", async () => {
    go.GetLibrarySummary.mockResolvedValue(null);
    await expect(getLibrarySummary()).resolves.toBeNull();
  });

  it("maps a library summary", async () => {
    go.GetLibrarySummary.mockResolvedValue({
      root: "/m",
      total: 3,
      totalSize: 99,
      counts: { video: 2, audio: 0, image: 1 },
    });
    await expect(getLibrarySummary()).resolves.toStrictEqual({
      root: "/m",
      total: 3,
      totalSize: 99,
      counts: { video: 2, audio: 0, image: 1 },
    });
  });
});

describe("chooseFolder", () => {
  it("resolves with nothing: results arrive as events", async () => {
    go.ChooseFolder.mockResolvedValue(undefined);
    await expect(chooseFolder()).resolves.toBeUndefined();
  });

  it("propagates Go's error", async () => {
    go.ChooseFolder.mockRejectedValue("scan failed");
    await expect(chooseFolder()).rejects.toBe("scan failed");
  });
});
