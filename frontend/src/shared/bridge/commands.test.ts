import { beforeEach, describe, expect, it, vi } from "vitest";

const go = vi.hoisted(() => ({
  AppInfo: vi.fn(),
  ChooseFolder: vi.fn(),
  GetLibrarySummary: vi.fn(),
  GetServerState: vi.fn(),
  GetLogHistory: vi.fn(),
  StartServer: vi.fn(),
  StopServer: vi.fn(),
}));
vi.mock("../../../wailsjs/go/bridge/App", () => go);

import {
  chooseFolder,
  getAppInfo,
  getLibrarySummary,
  getLogHistory,
  getServerState,
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
    go.GetServerState.mockResolvedValue({ state: "running", name: "Den", addr: "127.0.0.1:8765", error: "" });
    await expect(getServerState()).resolves.toStrictEqual({
      state: "running",
      name: "Den",
      addr: "127.0.0.1:8765",
      error: "",
    });
  });

  it("getServerState rejects a state the frontend does not know", async () => {
    go.GetServerState.mockResolvedValue({ state: "paused", name: "", addr: "", error: "" });
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
