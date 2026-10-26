import { useCallback, useState } from "react";
import { checkPort, freePort, type PortHolder } from "@/shared/bridge";

interface PortTools {
  /** Null until a check has been made. Empty means the checked port is free. */
  holders: PortHolder[] | null;
  /** The port the list above is about; the text field may have moved on since. */
  checkedPort: number | null;
  /** Set after a program was ended and the port is free again. */
  freedNote: string | null;
  pending: boolean;
  /** Go's reason the last command was refused. */
  error: string | null;
  check: (port: number) => Promise<void>;
  free: (port: number, holder: PortHolder) => Promise<void>;
  reset: () => void;
}

/**
 * Looks at a port and ends the program holding it. Nothing here is cached
 * truth: every result comes from Go, and a program is only ended by naming the
 * process id Go just reported.
 */
export function usePortTools(): PortTools {
  const [holders, setHolders] = useState<PortHolder[] | null>(null);
  const [checkedPort, setCheckedPort] = useState<number | null>(null);
  const [freedNote, setFreedNote] = useState<string | null>(null);
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

  const check = useCallback(
    (port: number) =>
      run(async () => {
        setFreedNote(null);
        setHolders(await checkPort(port));
        setCheckedPort(port);
      }),
    [run],
  );

  const free = useCallback(
    (port: number, holder: PortHolder) =>
      run(async () => {
        await freePort(port, holder.pid);
        setFreedNote(`${holder.name || `Process ${holder.pid}`} was ended. Port ${port} is free.`);
        setHolders(await checkPort(port));
        setCheckedPort(port);
      }),
    [run],
  );

  const reset = useCallback(() => {
    setHolders(null);
    setCheckedPort(null);
    setFreedNote(null);
    setError(null);
  }, []);

  return { holders, checkedPort, freedNote, pending, error, check, free, reset };
}
