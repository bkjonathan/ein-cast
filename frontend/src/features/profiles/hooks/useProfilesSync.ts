import { useEffect } from "react";
import { listProfiles } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useProfilesStore } from "../store";

/** Keeps the profiles store in step with Go. Mount once, near the root. */
export function useProfilesSync(): void {
  const apply = useProfilesStore((s) => s.apply);
  const hydrate = useProfilesStore((s) => s.hydrate);

  useWailsEvent("profiles:changed", apply);

  useEffect(() => {
    listProfiles()
      .then(hydrate)
      // No Go runtime (plain-browser dev): the profiles panel stays empty.
      .catch(() => undefined);
  }, [hydrate]);
}
