import { useCallback, useState } from "react";
import { rotateAccessToken, updateSettings, type SettingsInput } from "@/shared/bridge";

interface SettingsCommands {
  save: (input: SettingsInput) => Promise<boolean>;
  rotate: () => Promise<void>;
  pending: boolean;
  /** Go's reason the last command was refused, e.g. a validation message. */
  error: string | null;
}

/**
 * Sends settings commands. The saved values come back through the
 * "settings:changed" event, so the screen shows what Go actually stored.
 */
export function useSettingsCommands(): SettingsCommands {
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

  const save = useCallback((input: SettingsInput) => run(() => updateSettings(input)), [run]);
  const rotate = useCallback(async () => {
    await run(rotateAccessToken);
  }, [run]);

  return { save, rotate, pending, error };
}
