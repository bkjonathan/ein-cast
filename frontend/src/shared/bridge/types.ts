/**
 * Frontend-owned view of what Go sends. We map the generated wailsjs classes
 * into these plain types in commands.ts so the rest of the UI never depends on
 * generated code (the JS-side anti-corruption layer).
 */
export interface AppInfo {
  name: string;
  version: string;
  platform: string;
}

/** What kind of file an item is. The same three keys count them in KindCounts. */
export type MediaKind = "video" | "audio" | "image";

/**
 * One file in the library. It has no disk path: devices and this UI refer to a
 * file by `id` alone.
 */
export interface LibraryItem {
  id: string;
  /** The library the file is in. */
  libraryId: string;
  name: string;
  /** Slash-separated, relative to the library folder. */
  relPath: string;
  kind: MediaKind;
  size: number;
  /** Last modified, Unix milliseconds. */
  modTime: number;
  /** What to show: read from the name or a .nfo, else the file name. `name` stays the file name. */
  title: string;
  /** Release year, 0 when unknown. */
  year: number;
  /** The series an episode belongs to; empty for everything else. */
  show: string;
  /** 0 when unknown. */
  season: number;
  episode: number;
  /** True when the thumbnail is a poster found next to the file, not a frame of it. */
  poster: boolean;
}

export interface KindCounts {
  video: number;
  audio: number;
  image: number;
}

/**
 * One library: a folder the person added, what is in it, and what is happening
 * to it. File paths never cross the bridge; `root` is the folder itself, which
 * the person chose and sees.
 */
export interface LibraryState {
  id: string;
  /** What it is called; starts as the folder's name and can be changed. */
  name: string;
  root: string;
  total: number;
  totalSize: number;
  counts: KindCounts;
  /** A scan the person asked for is running. Quiet rescans after a change on disk do not set it. */
  scanning: boolean;
  /** A short, user-facing reason the last scan failed; empty if it did not. */
  error: string;
  /** True while Go watches the folder, so changes on disk reach the library by themselves. */
  watching: boolean;
  /** Kept off smart TVs: a TV cannot say who is watching, so this is the one choice it gets. */
  hiddenFromTVs: boolean;
}

/**
 * One person's view of the library. `libraries` matter only when `allLibraries`
 * is false; empty `kinds` means every kind; `hiddenFolders` are names (with * and ?)
 * matched against each folder in a file's path. A PIN never leaves Go: only
 * whether one is set.
 */
export interface Profile {
  /** Empty for a profile that has not been saved yet. */
  id: string;
  name: string;
  color: string;
  allLibraries: boolean;
  libraries: string[];
  kinds: MediaKind[];
  hiddenFolders: string[];
  hasPin: boolean;
}

/** The user's preferences, plus the access token when it is switched on. */
export interface Settings {
  port: number;
  name: string;
  requireToken: boolean;
  /** Smart TV (DLNA) discovery. Never on together with requireToken: Go refuses both. */
  dlna: boolean;
  /** Start the server when EinCast opens and right after the first folder is added. */
  autoStart: boolean;
  /** Keep the computer from sleeping while the server runs. */
  keepAwake: boolean;
  /** Open EinCast when the person signs in to the computer. */
  openAtLogin: boolean;
  /** Closing the window keeps sharing; read when EinCast starts. */
  hideOnClose: boolean;
  /** Where the person said ffmpeg is; empty means "look for it". */
  ffmpegPath: string;
  /** Empty unless requireToken is on. */
  accessToken: string;
  /** False when the token could not be saved to the system credential store. */
  tokenPersisted: boolean;
}

/** The editable part of Settings. */
export type SettingsInput = Pick<
  Settings,
  "port" | "name" | "requireToken" | "dlna" | "autoStart" | "keepAwake" | "openAtLogin" | "hideOnClose"
>;

/** Whether ffmpeg was found, and what the person can do about it. */
export interface FFmpegStatus {
  available: boolean;
  /** Where it was found; empty when it was not. */
  path: string;
  /** The location the person set, shown even when it no longer works. */
  chosen: string;
  /** A one-line install command for this system; empty when there is none to suggest. */
  installCommand: string;
}

/** A program using a TCP port. `self` is EinCast's own process, which cannot be ended from here. */
export interface PortHolder {
  pid: number;
  /** The program's name; empty when the system would not say. */
  name: string;
  self: boolean;
}

export type ServerState = "idle" | "starting" | "running" | "stopping" | "failed";

/** The HTTP server's lifecycle as Go reports it. */
export interface ServerStatus {
  state: ServerState;
  name: string;
  /** Port in use while running, otherwise 0. */
  port: number;
  /** Reaches the server from this computer, e.g. "http://127.0.0.1:8765". */
  localUrl: string;
  /** Reach it from other devices, most likely first. Empty if there is no network. */
  lanUrls: string[];
  /** Friendlier addresses announced on the network, such as http://eincast.local:8765. */
  nameUrls: string[];
  /** What is announcing the server on the network: "mdns", "dlna". */
  discovery: string[];
  /** Failure reason while failed, otherwise empty. */
  error: string;
}

/** Remote access reuses the server's lifecycle states. */
export type RemoteState = ServerState;

/** Remote access (a tunnel) as Go reports it. */
export interface RemoteStatus {
  state: RemoteState;
  /** The provider in use; empty when none is. */
  provider: string;
  /**
   * True while anyone on the Internet might reach the library. It stays true
   * until Go has confirmed the tunnel stopped, so the warning never clears early.
   */
  public: boolean;
  /** The remote address while running. */
  url: string;
  /** Why remote access failed while failed. */
  error: string;
  /** The provider's own state: anything but "running" while it reconnects. */
  live: RemoteState;
  /** Whether an access PIN exists. */
  pinSet: boolean;
  /** Remote browsers currently signed in. */
  sessions: number;
  /** Providers that cannot be turned on until the person supplies what they need. */
  needsSetup: string[];
}

/**
 * How the UI asks for a setting: "text" is shown back; "secret" is write-only
 * (once saved, Go reports only that it is set); "file" is a path, with a picker.
 */
export type RemoteFieldKind = "text" | "secret" | "file";

/** One setting a provider needs from the person. */
export interface RemoteField {
  key: string;
  label: string;
  help: string;
  placeholder: string;
  kind: RemoteFieldKind;
  required: boolean;
}

/** Whether a setting is saved. `value` is always empty for a secret. */
export interface RemoteFieldState {
  key: string;
  set: boolean;
  value: string;
}

/** One way of reaching the library from outside. Names and advice live in the UI. */
export interface RemoteProvider {
  id: string;
  /** Anyone on the Internet can reach a public provider's address; it needs the PIN. */
  public: boolean;
  /** What the provider needs from the person; empty if nothing. */
  fields: RemoteField[];
}

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogAttr {
  key: string;
  value: string;
}

/** One log record. `seq` orders lines and lets history join the live feed. */
export interface LogLine {
  seq: number;
  /** Unix milliseconds. */
  time: number;
  level: LogLevel;
  /** The component that logged: "http", "library", "server", "app", ... */
  source: string;
  message: string;
  attrs: LogAttr[];
}
