import { useCallback, useState } from "react";
import {
  chooseFolder,
  removeLibrary,
  renameLibrary,
  rescanAllLibraries,
  rescanLibrary,
} from "@/shared/bridge";
import { useLibraryStore } from "../store";

interface LibraryActions {
  /** Opens the native folder picker, then adds the choice as a library. */
  add: () => Promise<void>;
  /** Scans one library again. */
  rescan: (id: string) => Promise<void>;
  /** Scans every library again. */
  rescanAll: () => Promise<void>;
  /** Takes a library out of the list; its folder on disk is untouched. */
  remove: (id: string) => Promise<void>;
  /** Renames a library; resolves true if Go accepted the name. */
  rename: (id: string, name: string) => Promise<boolean>;
  /** True while a command is in flight or Go reports a scan running. */
  busy: boolean;
  /** Why the last command was refused, in Go's words (for example, the folder is already added). */
  error: string | null;
}

/**
 * Library commands. They never assume success: results arrive through the
 * "library:scanned" and "library:status" events, which the store projects.
 */
export function useLibraryActions(): LibraryActions {
  const scanning = useLibraryStore((s) => s.libraries.some((l) => l.scanning));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(async (command: () => Promise<void>): Promise<boolean> => {
    setPending(true);
    setError(null);
    try {
      await command();
      return true;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return false;
    } finally {
      setPending(false);
    }
  }, []);

  const add = useCallback(async () => void (await run(chooseFolder)), [run]);
  const rescan = useCallback(async (id: string) => void (await run(() => rescanLibrary(id))), [run]);
  const rescanAll = useCallback(async () => void (await run(rescanAllLibraries)), [run]);
  const remove = useCallback(async (id: string) => void (await run(() => removeLibrary(id))), [run]);
  const rename = useCallback((id: string, name: string) => run(() => renameLibrary(id, name)), [run]);

  return { add, rescan, rescanAll, remove, rename, busy: pending || scanning, error };
}
