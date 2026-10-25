import { useEffect } from "react";
import { getLibraryStatus, getLibrarySummary } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { idleStatus, useLibraryStore } from "../store";

/** Keeps the library store in step with Go. Mount once, near the root. */
export function useLibrarySync(): void {
  const applyScanned = useLibraryStore((s) => s.applyScanned);
  const applyStatus = useLibraryStore((s) => s.applyStatus);
  const hydrate = useLibraryStore((s) => s.hydrate);

  useWailsEvent("library:scanned", applyScanned);
  useWailsEvent("library:status", applyStatus);

  useEffect(() => {
    Promise.all([getLibrarySummary(), getLibraryStatus()])
      .then(([summary, status]) => hydrate(summary, status))
      // No Go runtime (plain-browser dev): behave as "nothing scanned yet".
      .catch(() => hydrate(null, idleStatus));
  }, [hydrate]);
}
