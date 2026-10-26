import { useCallback, useEffect, useState } from "react";
import {
  chooseFile,
  clearRemoteProviderField,
  getRemoteProviderConfig,
  setRemoteProviderField,
  type RemoteFieldState,
} from "@/shared/bridge";

interface ProviderConfig {
  /** Null until Go has answered. */
  states: RemoteFieldState[] | null;
  save: (key: string, value: string) => Promise<boolean>;
  remove: (key: string) => Promise<boolean>;
  /** Opens the file picker; resolves with the chosen path, or "" if cancelled. */
  browse: (title: string) => Promise<string>;
  /** The setting being changed right now, if any. */
  busyKey: string | null;
  /** Go's reason the last change was refused, e.g. "Hostname: enter the hostname only". */
  error: string | null;
}

/**
 * A provider's saved settings. Secrets arrive as "set or not", never with their
 * value. After each change the states are read again from Go, so the form shows
 * what was actually stored (Go cleans values: a pasted URL becomes a host name).
 * `refreshOn` re-reads them when something else may have changed them.
 */
export function useProviderConfig(providerId: string, refreshOn: unknown): ProviderConfig {
  const [states, setStates] = useState<RemoteFieldState[] | null>(null);
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let current = true; // ignore an answer for a provider that is no longer shown
    setStates(null);
    setError(null);
    getRemoteProviderConfig(providerId)
      .then((s) => current && setStates(s))
      // No Go runtime (plain-browser dev): the form stays empty.
      .catch(() => undefined);
    return () => {
      current = false;
    };
  }, [providerId, refreshOn]);

  const change = useCallback(
    async (key: string, command: () => Promise<void>): Promise<boolean> => {
      setBusyKey(key);
      setError(null);
      try {
        await command();
        setStates(await getRemoteProviderConfig(providerId));
        return true;
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
        return false;
      } finally {
        setBusyKey(null);
      }
    },
    [providerId],
  );

  const save = useCallback(
    (key: string, value: string) => change(key, () => setRemoteProviderField(providerId, key, value)),
    [change, providerId],
  );
  const remove = useCallback(
    (key: string) => change(key, () => clearRemoteProviderField(providerId, key)),
    [change, providerId],
  );
  const browse = useCallback(async (title: string) => {
    try {
      return await chooseFile(title);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return "";
    }
  }, []);

  return { states, save, remove, browse, busyKey, error };
}
