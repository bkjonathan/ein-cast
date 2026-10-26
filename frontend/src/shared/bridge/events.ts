// A bridge module: allowed to import ../wailsjs/runtime.
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import {
  toLibraryState,
  toLogLine,
  toRemoteStatus,
  toServerStatus,
  type RawLibraryState,
  type RawLogLine,
  type RawRemoteStatus,
  type RawServerStatus,
} from "./mappers";
import type {
  LibraryState,
  LogLine,
  RemoteStatus,
  ServerStatus,
  Settings,
} from "./types";

/** Every event Go emits, with its payload. Mirrors internal/bridge/events.go. */
export interface EventMap {
  "library:scanned": LibraryState[];
  "server:state": ServerStatus;
  "log:line": LogLine;
  "library:status": LibraryState[];
  "settings:changed": Settings;
  "remote:status": RemoteStatus;
}

export type EventName = keyof EventMap;

// The runtime hands payloads over untyped. Each decoder turns the wire shape
// into the frontend type; Go is the only producer, so plain DTOs are asserted
// and anything with a narrowed field goes through a checked mapper.
const decoders: { [K in EventName]: (raw: unknown) => EventMap[K] } = {
  "library:scanned": (raw) => (raw as RawLibraryState[]).map(toLibraryState),
  "server:state": (raw) => toServerStatus(raw as RawServerStatus),
  "log:line": (raw) => toLogLine(raw as RawLogLine),
  "library:status": (raw) => (raw as RawLibraryState[]).map(toLibraryState),
  "settings:changed": (raw) => raw as Settings,
  "remote:status": (raw) => toRemoteStatus(raw as RawRemoteStatus),
};

/** Subscribes to a Go event. Returns the unsubscribe function. */
export function onEvent<K extends EventName>(
  name: K,
  handler: (payload: EventMap[K]) => void,
): () => void {
  return EventsOn(name, (raw: unknown) => handler(decoders[name](raw)));
}
