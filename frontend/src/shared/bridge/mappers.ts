import type {
  LibraryItem,
  LogLevel,
  LogLine,
  MediaKind,
  RemoteField,
  RemoteFieldKind,
  RemoteProvider,
  RemoteStatus,
  ServerState,
  ServerStatus,
} from "./types";

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
function toState(raw: string, what: string): ServerState {
  const state = serverStates.find((s) => s === raw);
  if (!state) throw new Error(`unknown ${what} state from Go: "${raw}"`);
  return state;
}

export function toServerStatus(raw: RawServerStatus): ServerStatus {
  return {
    state: toState(raw.state, "server"),
    name: raw.name,
    port: raw.port,
    localUrl: raw.localUrl,
    lanUrls: [...raw.lanUrls],
    error: raw.error,
  };
}

/** The wire shape of remote access, before its states are narrowed. */
export interface RawRemoteStatus {
  state: string;
  provider: string;
  public: boolean;
  url: string;
  error: string;
  live: string;
  pinSet: boolean;
  sessions: number;
  needsSetup: string[];
}

export function toRemoteStatus(raw: RawRemoteStatus): RemoteStatus {
  return {
    state: toState(raw.state, "remote access"),
    provider: raw.provider,
    public: raw.public,
    url: raw.url,
    error: raw.error,
    live: toState(raw.live, "remote access"),
    pinSet: raw.pinSet,
    sessions: raw.sessions,
    needsSetup: [...raw.needsSetup],
  };
}

/** The wire shape of a provider, before its fields' kinds are narrowed. */
export interface RawRemoteProvider {
  id: string;
  public: boolean;
  fields: {
    key: string;
    label: string;
    help: string;
    placeholder: string;
    kind: string;
    required: boolean;
  }[];
}

const fieldKinds: readonly RemoteFieldKind[] = ["text", "secret", "file"];

/**
 * An unfamiliar field kind is treated as a secret, not rejected and not shown as
 * text: a setting the UI does not understand is safest as write-only.
 */
function toField(raw: RawRemoteProvider["fields"][number]): RemoteField {
  return {
    key: raw.key,
    label: raw.label,
    help: raw.help,
    placeholder: raw.placeholder,
    kind: fieldKinds.find((k) => k === raw.kind) ?? "secret",
    required: raw.required,
  };
}

export function toRemoteProvider(raw: RawRemoteProvider): RemoteProvider {
  return { id: raw.id, public: raw.public, fields: raw.fields.map(toField) };
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

const mediaKinds: readonly MediaKind[] = ["video", "audio", "image"];

/** The wire shape of a library item, before `kind` is narrowed. */
export interface RawLibraryItem {
  id: string;
  name: string;
  relPath: string;
  kind: string;
  size: number;
  modTime: number;
}

/**
 * Like the server state, an unknown kind means the two sides disagree about the
 * protocol; hiding the file would be worse than failing loudly.
 */
export function toLibraryItem(raw: RawLibraryItem): LibraryItem {
  const kind = mediaKinds.find((k) => k === raw.kind);
  if (!kind) throw new Error(`unknown media kind from Go: "${raw.kind}"`);
  return {
    id: raw.id,
    name: raw.name,
    relPath: raw.relPath,
    kind,
    size: raw.size,
    modTime: raw.modTime,
  };
}
