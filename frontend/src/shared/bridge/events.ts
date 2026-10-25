// The only module in the frontend allowed to import ../wailsjs/runtime.
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import { toLogLine, toServerStatus, type RawLogLine, type RawServerStatus } from "./mappers";
import type { LibrarySummary, LogLine, ServerStatus } from "./types";

/** Every event Go emits, with its payload. Mirrors internal/bridge/events.go. */
export interface EventMap {
  "library:scanned": LibrarySummary;
  "server:state": ServerStatus;
  "log:line": LogLine;
}

export type EventName = keyof EventMap;

// The runtime hands payloads over untyped. Each decoder turns the wire shape
// into the frontend type; Go is the only producer, so plain DTOs are asserted
// and anything with a narrowed field goes through a checked mapper.
const decoders: { [K in EventName]: (raw: unknown) => EventMap[K] } = {
  "library:scanned": (raw) => raw as LibrarySummary,
  "server:state": (raw) => toServerStatus(raw as RawServerStatus),
  "log:line": (raw) => toLogLine(raw as RawLogLine),
};

/** Subscribes to a Go event. Returns the unsubscribe function. */
export function onEvent<K extends EventName>(
  name: K,
  handler: (payload: EventMap[K]) => void,
): () => void {
  return EventsOn(name, (raw: unknown) => handler(decoders[name](raw)));
}
