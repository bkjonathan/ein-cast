import { useCallback, useEffect, useRef } from "react";
import { listLibraries, listLibrary } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useLibraryStore } from "../store";

/** Keeps the library store in step with Go. Mount once, near the root. */
export function useLibrarySync(): void {
  const apply = useLibraryStore((s) => s.apply);
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

  // The files changed: a scan found something new, or a library came or went.
  useWailsEvent("library:scanned", (libraries) => {
    apply(libraries);
    refreshItems();
  });
  // Only what is happening changed (scanning, watching, a rename): the files did not.
  useWailsEvent("library:status", apply);

  useEffect(() => {
    listLibraries()
      .then(hydrate)
      // No Go runtime (plain-browser dev): behave as "no libraries yet".
      .catch(() => hydrate([]));
    refreshItems();
  }, [hydrate, refreshItems]);
}
