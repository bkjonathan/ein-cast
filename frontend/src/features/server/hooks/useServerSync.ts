import { useEffect } from "react";
import { getServerState } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useServerStore } from "../store";

/** Keeps the server store in step with Go. Mount once, near the root. */
export function useServerSync(): void {
  const applyStatus = useServerStore((s) => s.applyStatus);
  const hydrate = useServerStore((s) => s.hydrate);

  useWailsEvent("server:state", applyStatus);

  useEffect(() => {
    getServerState()
      .then(hydrate)
      // No Go runtime (plain-browser dev): leave the status unknown.
      .catch(() => undefined);
  }, [hydrate]);
}
