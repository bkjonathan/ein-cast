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

export interface KindCounts {
  video: number;
  audio: number;
  image: number;
}

/** The library Go currently has loaded. File paths never cross the bridge. */
export interface LibrarySummary {
  root: string;
  total: number;
  totalSize: number;
  counts: KindCounts;
}

/** What the library scanner is doing, and why the last attempt failed. */
export interface LibraryStatus {
  scanning: boolean;
  /** The folder being scanned, or the last one attempted. */
  root: string;
  /** A short, user-facing reason the last scan failed; empty if it did not. */
  error: string;
}

/** The user's preferences, plus the access token when it is switched on. */
export interface Settings {
  port: number;
  name: string;
  requireToken: boolean;
  /** Empty unless requireToken is on. */
  accessToken: string;
  /** False when the token could not be saved to the system credential store. */
  tokenPersisted: boolean;
}

/** The editable part of Settings. */
export type SettingsInput = Pick<Settings, "port" | "name" | "requireToken">;

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
  /** Failure reason while failed, otherwise empty. */
  error: string;
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
