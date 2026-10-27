import { useCallback, useState } from "react";
import { checkPort, freePort, type PortHolder } from "@/shared/bridge";

interface PortTools {
  /** Null until a check has been made. Empty means the checked port is free. */
  holders: PortHolder[] | null;
  /** The port the list above is about; the text field may have moved on since. */
  checkedPort: number | null;
  /** Set after a program was ended and the port is free again. */
  /** Who was ended and which port that freed, to be put into words by the screen. */
  freed: { who: string | number; port: number } | null;
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
  const [freed, setFreed] = useState<{ who: string | number; port: number } | null>(null);
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
        setFreed(null);
        setHolders(await checkPort(port));
        setCheckedPort(port);
      }),
    [run],
  );

  const free = useCallback(
    (port: number, holder: PortHolder) =>
      run(async () => {
        await freePort(port, holder.pid);
        setFreed({ who: holder.name || holder.pid, port });
        setHolders(await checkPort(port));
        setCheckedPort(port);
      }),
    [run],
  );

  const reset = useCallback(() => {
    setHolders(null);
    setCheckedPort(null);
    setFreed(null);
    setError(null);
  }, []);

  return { holders, checkedPort, freed, pending, error, check, free, reset };
}
