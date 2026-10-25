import { useEffect, useRef } from "react";
import { onEvent, type EventMap, type EventName } from "@/shared/bridge";

/**
 * Subscribes to a Go event for the lifetime of the component. The latest
 * handler is always called, but a new handler identity does not resubscribe.
 */
export function useWailsEvent<K extends EventName>(
  name: K,
  handler: (payload: EventMap[K]) => void,
): void {
  const latest = useRef(handler);
  useEffect(() => {
    latest.current = handler;
  });

  useEffect(() => onEvent(name, (payload) => latest.current(payload)), [name]);
}
