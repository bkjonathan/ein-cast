import { useEffect } from "react";
import { getLibrarySummary } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useLibraryStore } from "../store";

/** Keeps the library store in step with Go. Mount once, near the root. */
export function useLibrarySync(): void {
  const applyScanned = useLibraryStore((s) => s.applyScanned);
  const hydrate = useLibraryStore((s) => s.hydrate);

  useWailsEvent("library:scanned", applyScanned);

  useEffect(() => {
    getLibrarySummary()
      .then(hydrate)
      // No Go runtime (plain-browser dev): behave as "nothing scanned yet".
      .catch(() => hydrate(null));
  }, [hydrate]);
}
