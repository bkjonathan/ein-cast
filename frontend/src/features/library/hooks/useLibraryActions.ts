import { useCallback, useState } from "react";
import { chooseFolder, rescanLibrary } from "@/shared/bridge";
import { useLibraryStore } from "../store";

interface LibraryActions {
  /** Opens the native folder picker, then scans the choice. */
  choose: () => Promise<void>;
  /** Scans the current folder again. */
  rescan: () => Promise<void>;
  /** True while a command is in flight or Go reports a scan running. */
  busy: boolean;
  /** Why the last command was refused (for example, the dialog failed). */
  error: string | null;
}

/**
 * Library commands. They never assume success: results arrive through the
 * "library:scanned" and "library:status" events, which the store projects.
 */
export function useLibraryActions(): LibraryActions {
  const scanning = useLibraryStore((s) => s.status?.scanning ?? false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(async (command: () => Promise<void>) => {
    setPending(true);
    setError(null);
    try {
      await command();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPending(false);
    }
  }, []);

  const choose = useCallback(() => run(chooseFolder), [run]);
  const rescan = useCallback(() => run(rescanLibrary), [run]);

  return { choose, rescan, busy: pending || scanning, error };
}
