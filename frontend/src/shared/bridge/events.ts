// The only module in the frontend allowed to import ../wailsjs/runtime.
import { EventsOn } from "../../../wailsjs/runtime/runtime";
import type { LibrarySummary } from "./types";

/** Every event Go emits, with its payload. Mirrors internal/bridge/events.go. */
export interface EventMap {
  "library:scanned": LibrarySummary;
}

export type EventName = keyof EventMap;

/** Subscribes to a Go event. Returns the unsubscribe function. */
export function onEvent<K extends EventName>(
  name: K,
  handler: (payload: EventMap[K]) => void,
): () => void {
  // The runtime hands payloads over untyped. Go is the only producer and emits
  // exactly the DTO shape declared in EventMap, so the assertion is safe.
  return EventsOn(name, (payload: EventMap[K]) => handler(payload));
}
