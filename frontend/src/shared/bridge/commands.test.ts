import { beforeEach, describe, expect, it, vi } from "vitest";

const go = vi.hoisted(() => ({
  AppInfo: vi.fn(),
  ChooseFolder: vi.fn(),
  GetLibrarySummary: vi.fn(),
  GetServerState: vi.fn(),
  GetLogHistory: vi.fn(),
  GetLibraryStatus: vi.fn(),
  ListLibrary: vi.fn(),
  GetSettings: vi.fn(),
  RescanLibrary: vi.fn(),
  RotateAccessToken: vi.fn(),
  UpdateSettings: vi.fn(),
  StartServer: vi.fn(),
  StopServer: vi.fn(),
  GetRemoteStatus: vi.fn(),
  ListRemoteProviders: vi.fn(),
  EnableRemote: vi.fn(),
  DisableRemote: vi.fn(),
  GetRemoteProviderConfig: vi.fn(),
  SetRemoteProviderField: vi.fn(),
  ClearRemoteProviderField: vi.fn(),
  ChooseFile: vi.fn(),
  SetWindowTheme: vi.fn(),
  GetThumbnail: vi.fn(),
  ThumbnailsAvailable: vi.fn(),
  SetRemotePIN: vi.fn(),
  ClearRemotePIN: vi.fn(),
}));
vi.mock("../../../wailsjs/go/bridge/App", () => go);

import {
  chooseFile,
  chooseFolder,
  clearRemotePin,
  clearRemoteProviderField,
  getRemoteProviderConfig,
  setRemoteProviderField,
  disableRemote,
  enableRemote,
  getRemoteStatus,
  listRemoteProviders,
  setRemotePin,
  setWindowTheme,
  getThumbnail,
  thumbnailsAvailable,
  getAppInfo,
  getLibrarySummary,
  getLibraryStatus,
  listLibrary,
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

describe("listLibrary", () => {
  it("maps every item and narrows the kind", async () => {
    go.ListLibrary.mockResolvedValue([
      { id: "a1", name: "a.mkv", relPath: "tv/a.mkv", kind: "video", size: 5, modTime: 9, extra: 1 },
    ]);
    await expect(listLibrary()).resolves.toStrictEqual([
      { id: "a1", name: "a.mkv", relPath: "tv/a.mkv", kind: "video", size: 5, modTime: 9 },
    ]);
  });

  it("is an empty list for an empty library", async () => {
    go.ListLibrary.mockResolvedValue([]);
    await expect(listLibrary()).resolves.toEqual([]);
  });
});

describe("library status and rescan", () => {
  it("getLibraryStatus maps the DTO", async () => {
    go.GetLibraryStatus.mockResolvedValue({ scanning: false, root: "/gone", error: "That folder no longer exists.", watching: true, extra: 1 });
    await expect(getLibraryStatus()).resolves.toStrictEqual({
      scanning: false,
      root: "/gone",
      error: "That folder no longer exists.",
      watching: true,
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

describe("remote access", () => {
  it("getRemoteStatus maps and narrows the DTO", async () => {
    go.GetRemoteStatus.mockResolvedValue({
      state: "running", provider: "tailscale", public: false, url: "http://den.ts.net:8765",
      error: "", live: "running", pinSet: true, sessions: 2, needsSetup: ["ngrok"], junk: 1,
    });
    await expect(getRemoteStatus()).resolves.toStrictEqual({
      state: "running", provider: "tailscale", public: false, url: "http://den.ts.net:8765",
      error: "", live: "running", pinSet: true, sessions: 2, needsSetup: ["ngrok"],
    });
  });

  it("getRemoteStatus rejects a state it does not know", async () => {
    go.GetRemoteStatus.mockResolvedValue({
      state: "hibernating", provider: "", public: false, url: "", error: "", live: "idle", pinSet: false, sessions: 0, needsSetup: [],
    });
    await expect(getRemoteStatus()).rejects.toThrow(/hibernating/);
  });

  it("listRemoteProviders keeps only what the UI needs", async () => {
    go.ListRemoteProviders.mockResolvedValue([
      {
        id: "cloudflare-named", public: true, x: 1,
        fields: [
          { key: "token", label: "Tunnel token", help: "h", placeholder: "p", kind: "secret", required: true, y: 2 },
          { key: "binary", label: "Location", help: "", placeholder: "", kind: "file", required: false },
        ],
      },
      { id: "tailscale", public: false, fields: [] },
    ]);
    await expect(listRemoteProviders()).resolves.toStrictEqual([
      {
        id: "cloudflare-named", public: true,
        fields: [
          { key: "token", label: "Tunnel token", help: "h", placeholder: "p", kind: "secret", required: true },
          { key: "binary", label: "Location", help: "", placeholder: "", kind: "file", required: false },
        ],
      },
      { id: "tailscale", public: false, fields: [] },
    ]);
  });

  it("enableRemote sends the provider id and surfaces Go's advice", async () => {
    go.EnableRemote.mockResolvedValueOnce(undefined);
    await enableRemote("tailscale");
    expect(go.EnableRemote).toHaveBeenCalledWith("tailscale");

    go.EnableRemote.mockRejectedValueOnce("Set an access PIN before turning on public access.");
    await expect(enableRemote("cloudflare-quick")).rejects.toMatch(/access PIN/);
  });

  it("disableRemote, setRemotePin and clearRemotePin forward to Go", async () => {
    go.DisableRemote.mockResolvedValue(undefined);
    go.SetRemotePIN.mockResolvedValue(undefined);
    go.ClearRemotePIN.mockResolvedValue(undefined);

    await expect(disableRemote()).resolves.toBeUndefined();
    await expect(setRemotePin("correct horse")).resolves.toBeUndefined();
    await expect(clearRemotePin()).resolves.toBeUndefined();
    expect(go.SetRemotePIN).toHaveBeenCalledWith("correct horse");
  });
});

describe("remote provider settings", () => {
  it("getRemoteProviderConfig maps the states", async () => {
    go.GetRemoteProviderConfig.mockResolvedValue([
      { key: "token", set: true, value: "", z: 1 },
      { key: "hostname", set: true, value: "media.example.com" },
    ]);
    await expect(getRemoteProviderConfig("cloudflare-named")).resolves.toStrictEqual([
      { key: "token", set: true, value: "" },
      { key: "hostname", set: true, value: "media.example.com" },
    ]);
    expect(go.GetRemoteProviderConfig).toHaveBeenCalledWith("cloudflare-named");
  });

  it("set and clear send the provider, the setting and (for set) the value, and surface Go's advice", async () => {
    go.SetRemoteProviderField.mockResolvedValueOnce(undefined);
    await setRemoteProviderField("cloudflare-named", "hostname", "media.example.com");
    expect(go.SetRemoteProviderField).toHaveBeenCalledWith("cloudflare-named", "hostname", "media.example.com");

    go.SetRemoteProviderField.mockRejectedValueOnce("Hostname: enter the hostname only, without a port");
    await expect(setRemoteProviderField("cloudflare-named", "hostname", "a:80")).rejects.toMatch(/hostname only/);

    go.ClearRemoteProviderField.mockResolvedValueOnce(undefined);
    await clearRemoteProviderField("cloudflare-named", "token");
    expect(go.ClearRemoteProviderField).toHaveBeenCalledWith("cloudflare-named", "token");
  });

  it("chooseFile resolves with the path, or an empty string when cancelled", async () => {
    go.ChooseFile.mockResolvedValueOnce("/opt/homebrew/bin/cloudflared");
    await expect(chooseFile("Choose cloudflared")).resolves.toBe("/opt/homebrew/bin/cloudflared");
    expect(go.ChooseFile).toHaveBeenCalledWith("Choose cloudflared");
    go.ChooseFile.mockResolvedValueOnce("");
    await expect(chooseFile("x")).resolves.toBe("");
  });
});

describe("setWindowTheme", () => {
  it("passes the mode through and propagates a refusal", async () => {
    go.SetWindowTheme.mockResolvedValueOnce(undefined);
    await expect(setWindowTheme("dark")).resolves.toBeUndefined();
    expect(go.SetWindowTheme).toHaveBeenCalledWith("dark");
    go.SetWindowTheme.mockRejectedValueOnce("unknown theme");
    await expect(setWindowTheme("sepia")).rejects.toBe("unknown theme");
  });
});

describe("thumbnails", () => {
  it("getThumbnail returns Go's data URL, or an empty string for none", async () => {
    go.GetThumbnail.mockResolvedValueOnce("data:image/jpeg;base64,/9g=");
    await expect(getThumbnail("a1")).resolves.toBe("data:image/jpeg;base64,/9g=");
    expect(go.GetThumbnail).toHaveBeenCalledWith("a1");
    go.GetThumbnail.mockResolvedValueOnce("");
    await expect(getThumbnail("a2")).resolves.toBe("");
  });

  it("thumbnailsAvailable returns Go's answer", async () => {
    go.ThumbnailsAvailable.mockResolvedValueOnce(false);
    await expect(thumbnailsAvailable()).resolves.toBe(false);
  });
});
