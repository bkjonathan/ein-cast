import { useCallback, useState } from "react";
import { chooseFolder } from "@/shared/bridge";

interface ChooseFolder {
  choose: () => Promise<void>;
  /** True while the dialog is open or the scan is running. */
  busy: boolean;
  error: string | null;
}

/**
 * Runs the native folder picker + scan. On success the store updates through
 * the "library:scanned" event, not through this hook.
 */
export function useChooseFolder(): ChooseFolder {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const choose = useCallback(async () => {
    setBusy(true);
    setError(null);
    try {
      await chooseFolder();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }, []);

  return { choose, busy, error };
}
