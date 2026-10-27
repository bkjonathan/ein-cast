import { useCallback, useState } from "react";
import { checkForUpdatesNow, openUpdatePage } from "@/shared/bridge";
import { useUpdatesStore } from "../store";

interface UpdateCommands {
  /** Asks for the newest version now, whatever the setting says. */
  check: () => Promise<void>;
  /** Opens the release page in the browser. */
  openPage: () => Promise<void>;
  /** Go's reason the last command was refused, in words for the person. */
  error: string | null;
}

/** Sends update commands. A failed check is not an error here: it is in the status. */
export function useUpdateCommands(): UpdateCommands {
  const apply = useUpdatesStore((s) => s.apply);
  const [error, setError] = useState<string | null>(null);

  const check = useCallback(async () => {
    setError(null);
    try {
      // The answer is Go's, so it is applied as it is; the same status also arrives as an event.
      apply(await checkForUpdatesNow());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [apply]);

  const openPage = useCallback(async () => {
    setError(null);
    try {
      await openUpdatePage();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  return { check, openPage, error };
}
