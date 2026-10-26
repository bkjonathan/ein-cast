// A bridge module: allowed to import ../wailsjs/go.
import {
  AppInfo as goAppInfo,
  ChooseFile as goChooseFile,
  ChooseFolder as goChooseFolder,
  ClearRemotePIN as goClearRemotePIN,
  ClearRemoteProviderField as goClearRemoteProviderField,
  DisableRemote as goDisableRemote,
  EnableRemote as goEnableRemote,
  GetLibraryStatus as goGetLibraryStatus,
  GetLibrarySummary as goGetLibrarySummary,
  GetLogHistory as goGetLogHistory,
  GetRemoteProviderConfig as goGetRemoteProviderConfig,
  GetRemoteStatus as goGetRemoteStatus,
  ListRemoteProviders as goListRemoteProviders,
  GetServerState as goGetServerState,
  GetThumbnail as goGetThumbnail,
  ListLibrary as goListLibrary,
  GetSettings as goGetSettings,
  RescanLibrary as goRescanLibrary,
  RotateAccessToken as goRotateAccessToken,
  SetRemotePIN as goSetRemotePIN,
  SetWindowTheme as goSetWindowTheme,
  SetRemoteProviderField as goSetRemoteProviderField,
  StartServer as goStartServer,
  StopServer as goStopServer,
  ThumbnailsAvailable as goThumbnailsAvailable,
  UpdateSettings as goUpdateSettings,
} from "../../../wailsjs/go/bridge/App";
import { toLibraryItem, toLogLine, toRemoteProvider, toRemoteStatus, toServerStatus } from "./mappers";
import type {
  AppInfo,
  LibraryItem,
  LibraryStatus,
  LibrarySummary,
  LogLine,
  RemoteFieldState,
  RemoteProvider,
  RemoteStatus,
  ServerStatus,
  Settings,
  SettingsInput,
} from "./types";

/** Asks Go for the app name, version and host platform. */
export async function getAppInfo(): Promise<AppInfo> {
  const dto = await goAppInfo();
  return { name: dto.name, version: dto.version, platform: dto.platform };
}

/**
 * Opens the native folder picker and scans the choice. Resolves when Go is done
 * (including when the user cancels); the new library arrives as a
 * "library:scanned" event, not as a return value. Rejects with Go's message.
 */
export async function chooseFolder(): Promise<void> {
  await goChooseFolder();
}

/** The library Go has loaded right now, or null before the first scan. */
export async function getLibrarySummary(): Promise<LibrarySummary | null> {
  // The generated type claims a non-null DTO, but Go returns nil (→ null) before
  // the first scan.
  const dto: Awaited<ReturnType<typeof goGetLibrarySummary>> | null =
    await goGetLibrarySummary();
  if (!dto) return null;
  return {
    root: dto.root,
    total: dto.total,
    totalSize: dto.totalSize,
    counts: { ...dto.counts },
  };
}

/** Every file in the library, in scan order. Re-ask after each "library:scanned". */
export async function listLibrary(): Promise<LibraryItem[]> {
  return (await goListLibrary()).map(toLibraryItem);
}

/**
 * Asks Go to start the HTTP server. Resolves when the request was accepted; the
 * outcome (running, or failed with a reason) arrives as a "server:state" event.
 * Rejects with Go's message if the request was refused.
 */
export async function startServer(): Promise<void> {
  await goStartServer();
}

/** Asks Go to stop the HTTP server gracefully. */
export async function stopServer(): Promise<void> {
  await goStopServer();
}

/** The server's current lifecycle state. */
export async function getServerState(): Promise<ServerStatus> {
  return toServerStatus(await goGetServerState());
}

/** Recent log lines, oldest first. Join them with live events by `seq`. */
export async function getLogHistory(): Promise<LogLine[]> {
  return (await goGetLogHistory()).map(toLogLine);
}

/** Scans the current library folder again. The result arrives as events. */
export async function rescanLibrary(): Promise<void> {
  await goRescanLibrary();
}

/** Whether a scan is running, and why the last one failed. */
export async function getLibraryStatus(): Promise<LibraryStatus> {
  const dto = await goGetLibraryStatus();
  return { scanning: dto.scanning, root: dto.root, error: dto.error, watching: dto.watching };
}

/** The saved settings, and the access token when it is on. */
export async function getSettings(): Promise<Settings> {
  const dto = await goGetSettings();
  return {
    port: dto.port,
    name: dto.name,
    requireToken: dto.requireToken,
    accessToken: dto.accessToken,
    tokenPersisted: dto.tokenPersisted,
  };
}

/**
 * Saves new settings. The saved values arrive as a "settings:changed" event.
 * Rejects with Go's validation message (e.g. "port must be between …").
 */
export async function updateSettings(input: SettingsInput): Promise<void> {
  await goUpdateSettings(input);
}

/** Replaces the access token; devices using the old link are locked out. */
export async function rotateAccessToken(): Promise<void> {
  await goRotateAccessToken();
}

/** Remote access as Go reports it right now. */
export async function getRemoteStatus(): Promise<RemoteStatus> {
  return toRemoteStatus(await goGetRemoteStatus());
}

/** The ways of reaching the library from outside that this build offers. */
export async function listRemoteProviders(): Promise<RemoteProvider[]> {
  return (await goListRemoteProviders()).map(toRemoteProvider);
}

/**
 * Turns a remote access provider on. Resolves once it is up; progress and the
 * outcome also arrive as "remote:status" events. It can take a while (a tunnel
 * must connect). Rejects with Go's advice, e.g. "Set an access PIN before …".
 */
export async function enableRemote(providerId: string): Promise<void> {
  await goEnableRemote(providerId);
}

/** Turns remote access off and signs every remote browser out. */
export async function disableRemote(): Promise<void> {
  await goDisableRemote();
}

/** Sets or changes the access PIN. Changing it signs every remote browser out. */
export async function setRemotePin(pin: string): Promise<void> {
  await goSetRemotePIN(pin);
}

/** Removes the access PIN. Go refuses while public access is on. */
export async function clearRemotePin(): Promise<void> {
  await goClearRemotePIN();
}

/**
 * Which of a provider's settings are saved. A secret comes back as set or not,
 * never with its value.
 */
export async function getRemoteProviderConfig(providerId: string): Promise<RemoteFieldState[]> {
  const states = await goGetRemoteProviderConfig(providerId);
  return states.map((s) => ({ key: s.key, set: s.set, value: s.value }));
}

/**
 * Saves one setting of a provider (a token, a host name). Rejects with Go's
 * advice, e.g. "Hostname: enter the hostname only, without a port", or if the
 * provider is on. The new state arrives as a "remote:status" event.
 */
export async function setRemoteProviderField(providerId: string, key: string, value: string): Promise<void> {
  await goSetRemoteProviderField(providerId, key, value);
}

/** Removes one saved setting of a provider. */
export async function clearRemoteProviderField(providerId: string, key: string): Promise<void> {
  await goClearRemoteProviderField(providerId, key);
}

/** Opens the native file picker. Resolves with the chosen path, or "" if cancelled. */
export async function chooseFile(title: string): Promise<string> {
  return goChooseFile(title);
}

/**
 * Tells the native window which chrome to use: "system", "light" or "dark".
 * Only Windows acts on it; elsewhere Go accepts and ignores it.
 */
export async function setWindowTheme(mode: string): Promise<void> {
  await goSetWindowTheme(mode);
}

/**
 * A small picture of a library file as a data URL for an <img>, or "" when it
 * has none (a song without cover art, or no ffmpeg on this computer). The first
 * request for a file can take a moment while Go makes the picture.
 */
export async function getThumbnail(id: string): Promise<string> {
  return goGetThumbnail(id);
}

/** Whether thumbnails can be made on this computer (that is, ffmpeg was found). */
export async function thumbnailsAvailable(): Promise<boolean> {
  return goThumbnailsAvailable();
}
