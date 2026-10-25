import type { LogLevel, LogLine, ServerState, ServerStatus } from "./types";

const serverStates: readonly ServerState[] = [
  "idle",
  "starting",
  "running",
  "stopping",
  "failed",
];

/** The wire shape of a server state, before `state` is narrowed. */
export interface RawServerStatus {
  state: string;
  name: string;
  port: number;
  localUrl: string;
  lanUrls: string[];
  error: string;
}

/**
 * Narrows Go's state string. An unknown value means the frontend and backend
 * disagree about the protocol, which should be loud, not silently "idle".
 */
export function toServerStatus(raw: RawServerStatus): ServerStatus {
  const state = serverStates.find((s) => s === raw.state);
  if (!state) throw new Error(`unknown server state from Go: "${raw.state}"`);
  return {
    state,
    name: raw.name,
    port: raw.port,
    localUrl: raw.localUrl,
    lanUrls: [...raw.lanUrls],
    error: raw.error,
  };
}

const logLevels: readonly LogLevel[] = ["debug", "info", "warn", "error"];

/** The wire shape of a log line, before `level` is narrowed. */
export interface RawLogLine {
  seq: number;
  time: number;
  level: string;
  source: string;
  message: string;
  attrs: { key: string; value: string }[];
}

/**
 * Unlike the server state, an unfamiliar log level is not worth throwing for:
 * dropping a line (or the whole history) would hide the very output that helps
 * debug the mismatch. It is shown as "info" instead.
 */
export function toLogLine(raw: RawLogLine): LogLine {
  return {
    seq: raw.seq,
    time: raw.time,
    level: logLevels.find((l) => l === raw.level) ?? "info",
    source: raw.source,
    message: raw.message,
    attrs: raw.attrs.map((a) => ({ key: a.key, value: a.value })),
  };
}
