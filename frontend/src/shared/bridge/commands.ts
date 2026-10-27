// A bridge module: allowed to import ../wailsjs/go.
import {
  AppInfo as goAppInfo,
  CheckPort as goCheckPort,
  ChooseFFmpeg as goChooseFFmpeg,
  ChooseFile as goChooseFile,
  CheckForUpdatesNow as goCheckForUpdatesNow,
  ChooseFolder as goChooseFolder,
  ClearFFmpegPath as goClearFFmpegPath,
  ClearProfilePIN as goClearProfilePIN,
  ClearRemotePIN as goClearRemotePIN,
  ClearRemoteProviderField as goClearRemoteProviderField,
  DeleteProfile as goDeleteProfile,
  DisableRemote as goDisableRemote,
  EnableRemote as goEnableRemote,
  FreePort as goFreePort,
  GetFFmpegStatus as goGetFFmpegStatus,
  GetLogHistory as goGetLogHistory,
  GetRemoteProviderConfig as goGetRemoteProviderConfig,
  GetRemoteStatus as goGetRemoteStatus,
  ListRemoteProviders as goListRemoteProviders,
  GetServerState as goGetServerState,
  GetThumbnail as goGetThumbnail,
  GetUpdateStatus as goGetUpdateStatus,
  ListLibraries as goListLibraries,
  ListProfiles as goListProfiles,
  SaveProfile as goSaveProfile,
  SetLibraryHiddenFromTVs as goSetLibraryHiddenFromTVs,
  SetProfilePIN as goSetProfilePIN,
  ListLibrary as goListLibrary,
  OpenUpdatePage as goOpenUpdatePage,
  RefreshAddresses as goRefreshAddresses,
  RemoveLibrary as goRemoveLibrary,
  RenameLibrary as goRenameLibrary,
  RescanAllLibraries as goRescanAllLibraries,
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
import {
  toLibraryItem,
  toLibraryState,
  toLogLine,
  toProfile,
  toRemoteProvider,
  toRemoteStatus,
  toServerStatus,
  toUpdateStatus,
} from "./mappers";
import type {
  AppInfo,
  FFmpegStatus,
  LibraryItem,
  LibraryState,
  LogLine,
  PortHolder,
  Profile,
  RemoteFieldState,
  RemoteProvider,
  RemoteStatus,
  ServerStatus,
  Settings,
  SettingsInput,
  UpdateStatus,
} from "./types";

/** Asks Go for the app name, version and host platform. */
export async function getAppInfo(): Promise<AppInfo> {
  const dto = await goAppInfo();
  return { name: dto.name, version: dto.version, platform: dto.platform };
}

/**
 * Opens the native folder picker and adds the choice as a library, scanning it.
 * Resolves when Go is done (including when the user cancels); the new library
 * arrives as "library:status" and "library:scanned" events, not as a return
 * value. Rejects with Go's message, e.g. that the folder is already in the library.
 */
export async function chooseFolder(): Promise<void> {
  await goChooseFolder();
}

/** Every library with what is in it and what is happening to it, in the order they were added. */
export async function listLibraries(): Promise<LibraryState[]> {
  return (await goListLibraries()).map(toLibraryState);
}

/** Takes a library out of the list. Its folder on disk is untouched. Rejects with Go's message. */
export async function removeLibrary(id: string): Promise<void> {
  await goRemoveLibrary(id);
}

/** Changes the name a library is shown as. Rejects with Go's message, e.g. an empty name. */
export async function renameLibrary(id: string, name: string): Promise<void> {
  await goRenameLibrary(id, name);
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

/** Scans one library again. The result arrives as events. */
export async function rescanLibrary(id: string): Promise<void> {
  await goRescanLibrary(id);
}

/** Scans every library again, one after another. Rejects if any failed. */
export async function rescanAllLibraries(): Promise<void> {
  await goRescanAllLibraries();
}

/** The saved settings, and the access token when it is on. */
export async function getSettings(): Promise<Settings> {
  const dto = await goGetSettings();
  return {
    port: dto.port,
    name: dto.name,
    requireToken: dto.requireToken,
    dlna: dto.dlna,
    autoStart: dto.autoStart,
    keepAwake: dto.keepAwake,
    openAtLogin: dto.openAtLogin,
    hideOnClose: dto.hideOnClose,
    ffmpegPath: dto.ffmpegPath,
    hardwareEncoding: dto.hardwareEncoding,
    checkForUpdates: dto.checkForUpdates,
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

/** Whether ffmpeg was found, where, and what to run to install it. */
export async function getFFmpegStatus(): Promise<FFmpegStatus> {
  const dto = await goGetFFmpegStatus();
  return { available: dto.available, path: dto.path, chosen: dto.chosen, installCommand: dto.installCommand };
}

/**
 * Opens the native file picker for the person to point at ffmpeg. Resolves when
 * Go is done (including when the person cancels); the result arrives as a
 * "settings:changed" event. Rejects with Go's reason when the file cannot be run.
 */
export async function chooseFFmpeg(): Promise<void> {
  await goChooseFFmpeg();
}

/** Goes back to looking for ffmpeg without help. */
export async function clearFFmpegPath(): Promise<void> {
  await goClearFFmpegPath();
}

/**
 * Restarts the running server so it uses the computer's current addresses. Remote
 * access goes down with it. Does nothing when the server is not running.
 */
export async function refreshAddresses(): Promise<void> {
  await goRefreshAddresses();
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

/**
 * The programs using a TCP port; an empty list means it is free. Only looks:
 * nothing is ended. Rejects with Go's advice, e.g. for a port out of range.
 */
export async function checkPort(port: number): Promise<PortHolder[]> {
  return (await goCheckPort(port)).map((h) => ({ pid: h.pid, name: h.name, self: h.self }));
}

/**
 * Ends the program with process id `pid`, which must be one `checkPort` just
 * reported for this port. Go asks it to quit and forces it only if it does not.
 * Resolves once the port is free. Rejects with Go's reason, e.g. that the
 * program belongs to another user.
 */
export async function freePort(port: number, pid: number): Promise<void> {
  await goFreePort(port, pid);
}

/** Keeps a library off smart TVs, or shows it to them again. The new state arrives as a "library:status" event. */
export async function setLibraryHiddenFromTVs(id: string, hidden: boolean): Promise<void> {
  await goSetLibraryHiddenFromTVs(id, hidden);
}

/** Every profile, with what each may see. Changes arrive as "profiles:changed". */
export async function listProfiles(): Promise<Profile[]> {
  return (await goListProfiles()).map(toProfile);
}

/**
 * Adds a profile (an empty id) or changes one, and resolves with it as stored. The PIN
 * is not part of it: see setProfilePin. Rejects with Go's reason in words for the person.
 */
export async function saveProfile(profile: Profile): Promise<Profile> {
  return toProfile(await goSaveProfile(profile));
}

/** Removes a profile and ends every sign-in made as it. Rejects if it is the last one. */
export async function deleteProfile(id: string): Promise<void> {
  await goDeleteProfile(id);
}

/** Sets or changes a profile's PIN (4 to 6 digits). Devices that chose the profile must choose again. */
export async function setProfilePin(id: string, pin: string): Promise<void> {
  await goSetProfilePIN(id, pin);
}

/** Removes a profile's PIN. */
export async function clearProfilePin(id: string): Promise<void> {
  await goClearProfilePIN(id);
}

/** Whether a newer version is out, as far as the last check knows. Changes arrive as "update:status". */
export async function getUpdateStatus(): Promise<UpdateStatus> {
  return toUpdateStatus(await goGetUpdateStatus());
}

/**
 * Asks for the newest version now, whatever the "check for updates" setting says.
 * A failed check is part of the returned status (`error`), not a rejection.
 */
export async function checkForUpdatesNow(): Promise<UpdateStatus> {
  return toUpdateStatus(await goCheckForUpdatesNow());
}

/**
 * Opens the newest release's download page in the browser. Go knows the address;
 * the UI cannot supply one. Rejects with Go's reason when there is nothing newer.
 */
export async function openUpdatePage(): Promise<void> {
  await goOpenUpdatePage();
}
