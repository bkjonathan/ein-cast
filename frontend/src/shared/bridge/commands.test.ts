import { beforeEach, describe, expect, it, vi } from "vitest";

const go = vi.hoisted(() => ({
  AppInfo: vi.fn(),
  CheckPort: vi.fn(),
  FreePort: vi.fn(),
  ChooseFolder: vi.fn(),
  ListLibraries: vi.fn(),
  RemoveLibrary: vi.fn(),
  RenameLibrary: vi.fn(),
  RescanAllLibraries: vi.fn(),
  GetServerState: vi.fn(),
  GetLogHistory: vi.fn(),
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
  GetFFmpegStatus: vi.fn(),
  ChooseFFmpeg: vi.fn(),
  ClearFFmpegPath: vi.fn(),
  RefreshAddresses: vi.fn(),
  ListProfiles: vi.fn(),
  SaveProfile: vi.fn(),
  DeleteProfile: vi.fn(),
  SetProfilePIN: vi.fn(),
  ClearProfilePIN: vi.fn(),
  SetLibraryHiddenFromTVs: vi.fn(),
  GetUpdateStatus: vi.fn(),
  CheckForUpdatesNow: vi.fn(),
  OpenUpdatePage: vi.fn(),
}));
vi.mock("../../../wailsjs/go/bridge/App", () => go);

import {
  checkForUpdatesNow,
  getUpdateStatus,
  openUpdatePage,
  checkPort,
  freePort,
  chooseFFmpeg,
  clearProfilePin,
  deleteProfile,
  listProfiles,
  saveProfile,
  setLibraryHiddenFromTVs,
  setProfilePin,
  chooseFile,
  chooseFolder,
  clearFFmpegPath,
  getFFmpegStatus,
  refreshAddresses,
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
  listLibraries,
  removeLibrary,
  renameLibrary,
  rescanAllLibraries,
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

describe("listLibraries", () => {
  it("is an empty list before any library is added", async () => {
    go.ListLibraries.mockResolvedValue([]);
    await expect(listLibraries()).resolves.toEqual([]);
  });

  it("maps every library and copies the counts", async () => {
    const dto = {
      id: "f1", name: "Films", root: "/m/films", total: 3, totalSize: 99,
      counts: { video: 2, audio: 0, image: 1 }, scanning: true, error: "", watching: true, hiddenFromTVs: true, extra: 1,
    };
    go.ListLibraries.mockResolvedValue([dto]);
    const got = await listLibraries();
    expect(got).toStrictEqual([
      {
        id: "f1", name: "Films", root: "/m/films", total: 3, totalSize: 99,
        counts: { video: 2, audio: 0, image: 1 }, scanning: true, error: "", watching: true, hiddenFromTVs: true,
      },
    ]);
    expect(got[0]?.counts).not.toBe(dto.counts);
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
      lanUrls: ["http://192.168.1.20:8765"], nameUrls: [], discovery: [],
      error: "",
    });
    await expect(getServerState()).resolves.toStrictEqual({
      state: "running",
      name: "Den",
      port: 8765,
      localUrl: "http://127.0.0.1:8765",
      lanUrls: ["http://192.168.1.20:8765"], nameUrls: [], discovery: [],
      error: "",
    });
  });

  it("getServerState rejects a state the frontend does not know", async () => {
    go.GetServerState.mockResolvedValue({ state: "paused", name: "", port: 0, localUrl: "", lanUrls: [], nameUrls: [], discovery: [], error: "" });
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
      {
        id: "a1", libraryId: "L1", name: "a.mkv", relPath: "tv/a.mkv", kind: "video", size: 5, modTime: 9,
        title: "Pilot", year: 2014, show: "Fargo", season: 1, episode: 1, poster: true, extra: 1,
      },
    ]);
    await expect(listLibrary()).resolves.toStrictEqual([
      {
        id: "a1", libraryId: "L1", name: "a.mkv", relPath: "tv/a.mkv", kind: "video", size: 5, modTime: 9,
        title: "Pilot", year: 2014, show: "Fargo", season: 1, episode: 1, poster: true,
      },
    ]);
  });

  it("is an empty list for an empty library", async () => {
    go.ListLibrary.mockResolvedValue([]);
    await expect(listLibrary()).resolves.toEqual([]);
  });
});

describe("library commands", () => {
  it("rescanLibrary sends the id, resolves with nothing and propagates a refusal", async () => {
    go.RescanLibrary.mockResolvedValueOnce(undefined);
    await expect(rescanLibrary("f1")).resolves.toBeUndefined();
    expect(go.RescanLibrary).toHaveBeenCalledWith("f1");
    go.RescanLibrary.mockRejectedValueOnce("that library is already being scanned");
    await expect(rescanLibrary("f1")).rejects.toBe("that library is already being scanned");
  });

  it("rescanAllLibraries resolves with nothing and propagates a failure", async () => {
    go.RescanAllLibraries.mockResolvedValueOnce(undefined);
    await expect(rescanAllLibraries()).resolves.toBeUndefined();
    go.RescanAllLibraries.mockRejectedValueOnce("one library failed");
    await expect(rescanAllLibraries()).rejects.toBe("one library failed");
  });

  it("removeLibrary and renameLibrary pass their arguments through", async () => {
    go.RemoveLibrary.mockResolvedValueOnce(undefined);
    await expect(removeLibrary("f1")).resolves.toBeUndefined();
    expect(go.RemoveLibrary).toHaveBeenCalledWith("f1");

    go.RenameLibrary.mockResolvedValueOnce(undefined);
    await expect(renameLibrary("f1", "Movie night")).resolves.toBeUndefined();
    expect(go.RenameLibrary).toHaveBeenCalledWith("f1", "Movie night");

    go.RenameLibrary.mockRejectedValueOnce("a library name must be 1 to 64 characters");
    await expect(renameLibrary("f1", "")).rejects.toMatch(/1 to 64/);
  });
});

describe("settings", () => {
  it("getSettings maps the DTO", async () => {
    go.GetSettings.mockResolvedValue({
      port: 9000, name: "Den", requireToken: true, dlna: false, autoStart: true, keepAwake: true,
      openAtLogin: false, hideOnClose: true, ffmpegPath: "/x/ffmpeg", hardwareEncoding: true, checkForUpdates: true, accessToken: "tok", tokenPersisted: true, junk: 1,
    });
    await expect(getSettings()).resolves.toStrictEqual({
      port: 9000, name: "Den", requireToken: true, dlna: false, autoStart: true, keepAwake: true,
      openAtLogin: false, hideOnClose: true, ffmpegPath: "/x/ffmpeg", hardwareEncoding: true, checkForUpdates: true, accessToken: "tok", tokenPersisted: true,
    });
  });

  it("updateSettings sends the editable fields and surfaces Go's validation message", async () => {
    const input = {
      port: 9100, name: "Den", requireToken: false, dlna: false,
      autoStart: true, keepAwake: true, openAtLogin: false, hideOnClose: false, hardwareEncoding: true, checkForUpdates: true,
    };
    go.UpdateSettings.mockResolvedValueOnce(undefined);
    await updateSettings(input);
    expect(go.UpdateSettings).toHaveBeenCalledWith(input);

    go.UpdateSettings.mockRejectedValueOnce("port must be between 1024 and 65535");
    await expect(updateSettings({ ...input, port: 80 })).rejects.toMatch(/between 1024/);
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

describe("port commands", () => {
  it("checkPort maps holders into plain objects", async () => {
    go.CheckPort.mockResolvedValue([{ pid: 300, name: "node", self: false, extra: 1 }]);
    await expect(checkPort(8765)).resolves.toStrictEqual([{ pid: 300, name: "node", self: false }]);
    expect(go.CheckPort).toHaveBeenCalledWith(8765);
  });

  it("checkPort of a free port is an empty list", async () => {
    go.CheckPort.mockResolvedValue([]);
    await expect(checkPort(8765)).resolves.toEqual([]);
  });

  it("freePort passes the port and the process, and resolves with nothing", async () => {
    go.FreePort.mockResolvedValue(undefined);
    await expect(freePort(8765, 300)).resolves.toBeUndefined();
    expect(go.FreePort).toHaveBeenCalledWith(8765, 300);
  });

  it("freePort propagates Go's reason", async () => {
    go.FreePort.mockRejectedValue("Not allowed: that program belongs to another user");
    await expect(freePort(8765, 300)).rejects.toMatch(/Not allowed/);
  });
});

describe("ffmpeg and addresses", () => {
  it("getFFmpegStatus maps the DTO", async () => {
    go.GetFFmpegStatus.mockResolvedValue({
      available: false, path: "", chosen: "/old/ffmpeg", installCommand: "brew install ffmpeg", junk: 1,
    });
    await expect(getFFmpegStatus()).resolves.toStrictEqual({
      available: false, path: "", chosen: "/old/ffmpeg", installCommand: "brew install ffmpeg",
    });
  });

  it("chooseFFmpeg resolves when cancelled and rejects with Go's reason for a file that cannot run", async () => {
    go.ChooseFFmpeg.mockResolvedValueOnce(undefined);
    await expect(chooseFFmpeg()).resolves.toBeUndefined();
    go.ChooseFFmpeg.mockRejectedValueOnce("/tmp/x is not executable");
    await expect(chooseFFmpeg()).rejects.toMatch(/not executable/);
  });

  it("clearFFmpegPath and refreshAddresses just ask Go", async () => {
    go.ClearFFmpegPath.mockResolvedValue(undefined);
    go.RefreshAddresses.mockResolvedValue(undefined);
    await clearFFmpegPath();
    await refreshAddresses();
    expect(go.ClearFFmpegPath).toHaveBeenCalledOnce();
    expect(go.RefreshAddresses).toHaveBeenCalledOnce();
  });
});

describe("profiles", () => {
  const dto = {
    id: "k1", name: "Kids", color: "green", allLibraries: false, libraries: ["a"], kinds: ["video"],
    hiddenFolders: ["18*"], hasPin: true, extra: 1,
  };
  const profile = {
    id: "k1", name: "Kids", color: "green", allLibraries: false, libraries: ["a"], kinds: ["video"] as const,
    hiddenFolders: ["18*"], hasPin: true,
  };

  it("listProfiles maps every profile and narrows the kinds", async () => {
    go.ListProfiles.mockResolvedValue([dto]);
    await expect(listProfiles()).resolves.toStrictEqual([{ ...profile, kinds: ["video"] }]);
  });

  it("saveProfile sends the profile and resolves with what Go stored", async () => {
    go.SaveProfile.mockResolvedValue({ ...dto, id: "new1" });
    const saved = await saveProfile({ ...profile, id: "", kinds: ["video"] });
    expect(go.SaveProfile).toHaveBeenCalledWith({ ...profile, id: "", kinds: ["video"] });
    expect(saved.id).toBe("new1");
  });

  it("saveProfile surfaces Go's reason in words", async () => {
    go.SaveProfile.mockRejectedValueOnce("the name must be 1 to 32 characters");
    await expect(saveProfile({ ...profile, name: "", kinds: ["video"] })).rejects.toMatch(/the name must be/);
  });

  it("the PIN commands pass the id and the PIN, and nothing else", async () => {
    go.SetProfilePIN.mockResolvedValue(undefined);
    go.ClearProfilePIN.mockResolvedValue(undefined);
    go.DeleteProfile.mockResolvedValue(undefined);
    go.SetLibraryHiddenFromTVs.mockResolvedValue(undefined);
    await setProfilePin("k1", "1234");
    await clearProfilePin("k1");
    await deleteProfile("k1");
    await setLibraryHiddenFromTVs("L1", true);
    expect(go.SetProfilePIN).toHaveBeenCalledWith("k1", "1234");
    expect(go.ClearProfilePIN).toHaveBeenCalledWith("k1");
    expect(go.DeleteProfile).toHaveBeenCalledWith("k1");
    expect(go.SetLibraryHiddenFromTVs).toHaveBeenCalledWith("L1", true);
  });
});

describe("updates", () => {
  const dto = {
    current: "1.1.1", checking: false, available: true, checkedAt: 1_700_000_000_000, error: "", junk: 1,
    latest: {
      version: "1.1.2", name: "EinCast 1.1.2", notes: "- fixes", publishedAt: 1_700_000_100_000,
      assets: [{ name: "a.zip", size: 5, sha256: "ab", extra: 1 }],
    },
  };
  const status = {
    current: "1.1.1", checking: false, available: true, checkedAt: 1_700_000_000_000, error: "",
    latest: {
      version: "1.1.2", name: "EinCast 1.1.2", notes: "- fixes", publishedAt: 1_700_000_100_000,
      assets: [{ name: "a.zip", size: 5, sha256: "ab" }],
    },
  };

  it("getUpdateStatus maps the DTO and drops unknown fields", async () => {
    go.GetUpdateStatus.mockResolvedValue(dto);
    await expect(getUpdateStatus()).resolves.toStrictEqual(status);
  });

  it("a status with no release has latest null, whether Go sent null or left it out", async () => {
    go.GetUpdateStatus.mockResolvedValueOnce({ ...dto, latest: null, available: false });
    await expect(getUpdateStatus()).resolves.toMatchObject({ latest: null });
    go.GetUpdateStatus.mockResolvedValueOnce({ ...dto, latest: undefined, available: false });
    await expect(getUpdateStatus()).resolves.toMatchObject({ latest: null });
  });

  it("checkForUpdatesNow resolves with the status, failures included", async () => {
    go.CheckForUpdatesNow.mockResolvedValue({ ...dto, latest: null, available: false, error: "offline" });
    await expect(checkForUpdatesNow()).resolves.toMatchObject({ error: "offline", latest: null });
  });

  it("openUpdatePage sends no address: Go knows which page to open", async () => {
    go.OpenUpdatePage.mockResolvedValueOnce(undefined);
    await openUpdatePage();
    expect(go.OpenUpdatePage).toHaveBeenCalledWith();

    go.OpenUpdatePage.mockRejectedValueOnce("there is no newer version to download");
    await expect(openUpdatePage()).rejects.toMatch(/no newer version/);
  });
});
