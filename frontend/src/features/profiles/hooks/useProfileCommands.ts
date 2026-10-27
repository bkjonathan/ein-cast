import { useCallback, useState } from "react";
import { clearProfilePin, deleteProfile, saveProfile, setProfilePin, type Profile } from "@/shared/bridge";

interface ProfileCommands {
  /** Resolves with the stored profile, or null if Go refused. */
  save: (profile: Profile) => Promise<Profile | null>;
  remove: (id: string) => Promise<boolean>;
  setPin: (id: string, pin: string) => Promise<boolean>;
  clearPin: (id: string) => Promise<boolean>;
  pending: boolean;
  /** Go's reason the last command was refused, in words for the person. */
  error: string | null;
  clearError: () => void;
}

/**
 * Sends profile commands. The new list comes back as a "profiles:changed" event,
 * so the screen shows what Go actually stored.
 */
export function useProfileCommands(): ProfileCommands {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(async <T,>(command: () => Promise<T>, fallback: T): Promise<T> => {
    setPending(true);
    setError(null);
    try {
      return await command();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return fallback;
    } finally {
      setPending(false);
    }
  }, []);

  const save = useCallback((p: Profile) => run<Profile | null>(() => saveProfile(p), null), [run]);
  const remove = useCallback((id: string) => run(async () => (await deleteProfile(id), true), false), [run]);
  const setPin = useCallback((id: string, pin: string) => run(async () => (await setProfilePin(id, pin), true), false), [run]);
  const clearPin = useCallback((id: string) => run(async () => (await clearProfilePin(id), true), false), [run]);
  const clearError = useCallback(() => setError(null), []);

  return { save, remove, setPin, clearPin, pending, error, clearError };
}
