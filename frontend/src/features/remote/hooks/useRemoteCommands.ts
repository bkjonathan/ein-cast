import { useCallback, useRef, useState } from "react";
import { clearRemotePin, disableRemote, enableRemote, setRemotePin } from "@/shared/bridge";

type Kind = "enable" | "disable" | "pin";

interface RemoteCommands {
  enable: (providerId: string) => Promise<void>;
  disable: () => Promise<void>;
  /** Resolves true when Go accepted the PIN. */
  setPin: (pin: string) => Promise<boolean>;
  clearPin: () => Promise<boolean>;
  /**
   * Which commands are in flight. Kept apart because turning on can take a
   * minute, and "turn off" must stay available meanwhile: it is how a slow
   * connection is cancelled.
   */
  busy: Record<Kind, boolean>;
  /** Go's reason the last command was refused. Lifecycle failures arrive as state. */
  error: string | null;
}

/**
 * Sends remote access commands. It never assumes they worked: what the screen
 * shows changes only when Go's "remote:status" event arrives.
 */
export function useRemoteCommands(): RemoteCommands {
  const [busy, setBusy] = useState<Record<Kind, boolean>>({ enable: false, disable: false, pin: false });
  const [error, setError] = useState<string | null>(null);
  // Set when the person cancels a start, so the start's resulting error
  // ("context canceled") is not shown as if something had gone wrong.
  const cancelled = useRef(false);

  const run = useCallback(async (kind: Kind, command: () => Promise<void>): Promise<boolean> => {
    setBusy((b) => ({ ...b, [kind]: true }));
    setError(null);
    try {
      await command();
      return true;
    } catch (err) {
      if (!(kind === "enable" && cancelled.current)) {
        setError(err instanceof Error ? err.message : String(err));
      }
      return false;
    } finally {
      setBusy((b) => ({ ...b, [kind]: false }));
    }
  }, []);

  const enable = useCallback(
    async (providerId: string) => {
      cancelled.current = false;
      await run("enable", () => enableRemote(providerId));
    },
    [run],
  );
  const disable = useCallback(async () => {
    cancelled.current = true;
    await run("disable", disableRemote);
  }, [run]);
  const setPin = useCallback((pin: string) => run("pin", () => setRemotePin(pin)), [run]);
  const clearPin = useCallback(() => run("pin", clearRemotePin), [run]);

  return { enable, disable, setPin, clearPin, busy, error };
}
