import { useEffect } from "react";
import { getUpdateStatus } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useUpdatesStore } from "../store";

/** Keeps the updates store in step with Go. Mount once, near the root. */
export function useUpdatesSync(): void {
  const apply = useUpdatesStore((s) => s.apply);
  const hydrate = useUpdatesStore((s) => s.hydrate);

  useWailsEvent("update:status", apply);

  useEffect(() => {
    getUpdateStatus()
      .then(hydrate)
      // No Go runtime (plain-browser dev): there is simply nothing to show.
      .catch(() => undefined);
  }, [hydrate]);
}
