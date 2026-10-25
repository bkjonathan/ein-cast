import { beforeEach, describe, expect, it, vi } from "vitest";

const go = vi.hoisted(() => ({
  AppInfo: vi.fn(),
  ChooseFolder: vi.fn(),
  GetLibrarySummary: vi.fn(),
  GetServerState: vi.fn(),
  GetLogHistory: vi.fn(),
  GetLibraryStatus: vi.fn(),
  GetSettings: vi.fn(),
  RescanLibrary: vi.fn(),
  RotateAccessToken: vi.fn(),
  UpdateSettings: vi.fn(),
  StartServer: vi.fn(),
  StopServer: vi.fn(),
}));
vi.mock("../../../wailsjs/go/bridge/App", () => go);

import {
  chooseFolder,
  getAppInfo,
  getLibrarySummary,
  getLibraryStatus,
  getLogHistory,
  getServerState,
  getSettings,
  rescanLibrary,
  rotateAccessToken,
  updateSettings,
  startServer,
  stopServer,
} from "./commands";

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

describe("server commands", () => {
  it("startServer / stopServer resolve with nothing: results arrive as events", async () => {
    go.StartServer.mockResolvedValue(undefined);
    go.StopServer.mockResolvedValue(undefined);
    await expect(startServer()).resolves.toBeUndefined();
    await expect(stopServer()).resolves.toBeUndefined();
  });

  it("startServer propagates a refusal", async () => {
    go.StartServer.mockRejectedValue("start: invalid state transition: running → starting");
    await expect(startServer()).rejects.toMatch(/invalid state transition/);
  });

  it("getServerState narrows the state string", async () => {
    go.GetServerState.mockResolvedValue({
      state: "running",
      name: "Den",
      port: 8765,
      localUrl: "http://127.0.0.1:8765",
      lanUrls: ["http://192.168.1.20:8765"],
      error: "",
    });
    await expect(getServerState()).resolves.toStrictEqual({
      state: "running",
      name: "Den",
      port: 8765,
      localUrl: "http://127.0.0.1:8765",
      lanUrls: ["http://192.168.1.20:8765"],
      error: "",
    });
  });

  it("getServerState rejects a state the frontend does not know", async () => {
    go.GetServerState.mockResolvedValue({ state: "paused", name: "", port: 0, localUrl: "", lanUrls: [], error: "" });
    await expect(getServerState()).rejects.toThrow(/unknown server state/);
  });
});

describe("getLogHistory", () => {
  it("maps every line and narrows levels", async () => {
    go.GetLogHistory.mockResolvedValue([
      { seq: 1, time: 10, level: "info", source: "app", message: "a", attrs: [] },
      { seq: 2, time: 20, level: "mystery", source: "http", message: "b", attrs: [{ key: "k", value: "v" }] },
    ]);
    await expect(getLogHistory()).resolves.toStrictEqual([
      { seq: 1, time: 10, level: "info", source: "app", message: "a", attrs: [] },
      { seq: 2, time: 20, level: "info", source: "http", message: "b", attrs: [{ key: "k", value: "v" }] },
    ]);
  });

  it("returns an empty list for an empty history", async () => {
    go.GetLogHistory.mockResolvedValue([]);
    await expect(getLogHistory()).resolves.toEqual([]);
  });
});

describe("library status and rescan", () => {
  it("getLibraryStatus maps the DTO", async () => {
    go.GetLibraryStatus.mockResolvedValue({ scanning: false, root: "/gone", error: "That folder no longer exists.", extra: 1 });
    await expect(getLibraryStatus()).resolves.toStrictEqual({
      scanning: false,
      root: "/gone",
      error: "That folder no longer exists.",
    });
  });

  it("rescanLibrary resolves with nothing and propagates a refusal", async () => {
    go.RescanLibrary.mockResolvedValueOnce(undefined);
    await expect(rescanLibrary()).resolves.toBeUndefined();
    go.RescanLibrary.mockRejectedValueOnce("a scan is already running");
    await expect(rescanLibrary()).rejects.toBe("a scan is already running");
  });
});

describe("settings", () => {
  it("getSettings maps the DTO", async () => {
    go.GetSettings.mockResolvedValue({
      port: 9000, name: "Den", requireToken: true, accessToken: "tok", tokenPersisted: true, junk: 1,
    });
    await expect(getSettings()).resolves.toStrictEqual({
      port: 9000, name: "Den", requireToken: true, accessToken: "tok", tokenPersisted: true,
    });
  });

  it("updateSettings sends the three editable fields and surfaces Go's validation message", async () => {
    go.UpdateSettings.mockResolvedValueOnce(undefined);
    await updateSettings({ port: 9100, name: "Den", requireToken: false });
    expect(go.UpdateSettings).toHaveBeenCalledWith({ port: 9100, name: "Den", requireToken: false });

    go.UpdateSettings.mockRejectedValueOnce("port must be between 1024 and 65535");
    await expect(updateSettings({ port: 80, name: "Den", requireToken: false })).rejects.toMatch(/between 1024/);
  });

  it("rotateAccessToken resolves with nothing", async () => {
    go.RotateAccessToken.mockResolvedValue(undefined);
    await expect(rotateAccessToken()).resolves.toBeUndefined();
  });
});
