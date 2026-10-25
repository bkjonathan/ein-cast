import { useEffect, useRef } from "react";
import { getLogHistory, type LogLine } from "@/shared/bridge";
import { createBatcher, type Batcher } from "@/shared/lib/batcher";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useLogStore } from "../store";

/** How long live lines are gathered before one store update. */
const FLUSH_MS = 50;

/** Keeps the log store in step with Go. Mount once, near the root. */
export function useLogSync(): void {
  const append = useLogStore((s) => s.append);
  const hydrate = useLogStore((s) => s.hydrate);
  const batcher = useRef<Batcher<LogLine> | null>(null);

  useWailsEvent("log:line", (line) => batcher.current?.push(line));

  useEffect(() => {
    const b = createBatcher(append, FLUSH_MS);
    batcher.current = b;

    getLogHistory()
      .then(hydrate)
      // No Go runtime (plain-browser dev): the viewer just starts empty.
      .catch(() => undefined);

    return () => {
      b.cancel();
      batcher.current = null;
    };
  }, [append, hydrate]);
}
