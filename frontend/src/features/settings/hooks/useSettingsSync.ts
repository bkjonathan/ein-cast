import { useEffect } from "react";
import { getSettings } from "@/shared/bridge";
import { useWailsEvent } from "@/shared/hooks/useWailsEvent";
import { useSettingsStore } from "../store";

/** Keeps the settings store in step with Go. Mount once, near the root. */
export function useSettingsSync(): void {
  const apply = useSettingsStore((s) => s.apply);
  const hydrate = useSettingsStore((s) => s.hydrate);

  useWailsEvent("settings:changed", apply);

  useEffect(() => {
    getSettings()
      .then(hydrate)
      // No Go runtime (plain-browser dev): the settings screen stays empty.
      .catch(() => undefined);
  }, [hydrate]);
}
