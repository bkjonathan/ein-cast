import { useCallback, useState } from "react";
import { resetApp } from "@/shared/bridge";
import { browserStorage, forgetWindowPreferences } from "../reset";

interface ResetCommand {
  reset: () => Promise<void>;
  /** True from the click on. Stays true when it works: the app is quitting. */
  pending: boolean;
  /** Go's reason the reset failed in part. */
  error: string | null;
}

/**
 * Resets EinCast. When Go succeeds it quits the app, so there is no "done" to
 * wait for and nothing to do afterwards; that is why this window forgets its own
 * preferences first. If Go then fails, the only loss is a theme or language the
 * person can pick again.
 */
export function useResetApp(): ResetCommand {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reset = useCallback(async () => {
    setPending(true);
    setError(null);
    forgetWindowPreferences(browserStorage());
    try {
      await resetApp();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setPending(false);
    }
  }, []);

  return { reset, pending, error };
}
