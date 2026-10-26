import { useEffect } from "react";
import { getRemoteStatus, listRemoteProviders } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useRemoteStore } from "../store";

/** Keeps the remote access store in step with Go. Mount once, near the root. */
export function useRemoteSync(): void {
  const applyStatus = useRemoteStore((s) => s.applyStatus);
  const hydrate = useRemoteStore((s) => s.hydrate);
  const setProviders = useRemoteStore((s) => s.setProviders);

  useWailsEvent("remote:status", applyStatus);

  useEffect(() => {
    getRemoteStatus()
      .then(hydrate)
      // No Go runtime (plain-browser dev): leave the status unknown.
      .catch(() => undefined);
    listRemoteProviders()
      .then(setProviders)
      .catch(() => undefined);
  }, [hydrate, setProviders]);
}
