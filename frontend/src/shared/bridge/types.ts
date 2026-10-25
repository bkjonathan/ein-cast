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

export type ServerState = "idle" | "starting" | "running" | "stopping" | "failed";

/** The HTTP server's lifecycle as Go reports it. */
export interface ServerStatus {
  state: ServerState;
  name: string;
  /** Bound "host:port" while running, otherwise empty. */
  addr: string;
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
