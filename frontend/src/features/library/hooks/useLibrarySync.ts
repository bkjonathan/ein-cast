import { useCallback, useEffect, useRef } from "react";
import { getLibraryStatus, getLibrarySummary, listLibrary } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { idleStatus, useLibraryStore } from "../store";

/** Keeps the library store in step with Go. Mount once, near the root. */
export function useLibrarySync(): void {
  const applyScanned = useLibraryStore((s) => s.applyScanned);
  const applyStatus = useLibraryStore((s) => s.applyStatus);
  const hydrate = useLibraryStore((s) => s.hydrate);
  const setItems = useLibraryStore((s) => s.setItems);

  // Two scans can finish close together; only the newest question's answer is
  // kept, so a slow older reply never overwrites a newer list.
  const asked = useRef(0);
  const refreshItems = useCallback(() => {
    const ticket = ++asked.current;
    listLibrary()
      .then((items) => {
        if (ticket === asked.current) setItems(items);
      })
      // No Go runtime (plain-browser dev): the list stays empty.
      .catch(() => undefined);
  }, [setItems]);

  useWailsEvent("library:scanned", (summary) => {
    applyScanned(summary);
    refreshItems();
  });
  useWailsEvent("library:status", applyStatus);

  useEffect(() => {
    Promise.all([getLibrarySummary(), getLibraryStatus()])
      .then(([summary, status]) => hydrate(summary, status))
      // No Go runtime (plain-browser dev): behave as "nothing scanned yet".
      .catch(() => hydrate(null, idleStatus));
    refreshItems();
  }, [hydrate, refreshItems]);
}
