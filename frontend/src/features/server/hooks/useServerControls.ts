import { useCallback, useState } from "react";
import { startServer, stopServer } from "@/shared/bridge";

interface ServerControls {
  start: () => Promise<void>;
  stop: () => Promise<void>;
  /** True while a command is in flight, to block double clicks. */
  pending: boolean;
  /** Why the last command was refused. Lifecycle failures arrive as state. */
  commandError: string | null;
}

/**
 * Sends start/stop commands. It never assumes they worked: the visible state
 * only changes when Go's "server:state" event arrives.
 */
export function useServerControls(): ServerControls {
  const [pending, setPending] = useState(false);
  const [commandError, setCommandError] = useState<string | null>(null);

  const run = useCallback(async (command: () => Promise<void>) => {
    setPending(true);
    setCommandError(null);
    try {
      await command();
    } catch (err) {
      setCommandError(err instanceof Error ? err.message : String(err));
    } finally {
      setPending(false);
    }
  }, []);

  const start = useCallback(() => run(startServer), [run]);
  const stop = useCallback(() => run(stopServer), [run]);

  return { start, stop, pending, commandError };
}
