import { useCallback, useEffect, useState } from "react";
import { chooseFFmpeg, clearFFmpegPath, getFFmpegStatus, type FFmpegStatus } from "@/shared/bridge";

interface FFmpegTools {
  /** Null until Go answers. */
  status: FFmpegStatus | null;
  pending: boolean;
  /** Go's reason the last command was refused, e.g. that the chosen file cannot run. */
  error: string | null;
  /** Asks again: ffmpeg may have been installed while EinCast was open. */
  recheck: () => Promise<void>;
  locate: () => Promise<void>;
  useAutomatic: () => Promise<void>;
}

/**
 * Where ffmpeg is. `refreshOn` is the saved location: when it changes, the answer
 * is asked for again, so choosing a file updates the screen without polling.
 */
export function useFFmpeg(refreshOn: string): FFmpegTools {
  const [status, setStatus] = useState<FFmpegStatus | null>(null);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const recheck = useCallback(async () => {
    try {
      setStatus(await getFFmpegStatus());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    void recheck();
  }, [recheck, refreshOn]);

  const run = useCallback(
    async (command: () => Promise<void>) => {
      setPending(true);
      setError(null);
      try {
        await command();
        await recheck();
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setPending(false);
      }
    },
    [recheck],
  );

  const locate = useCallback(() => run(chooseFFmpeg), [run]);
  const useAutomatic = useCallback(() => run(clearFFmpegPath), [run]);

  return { status, pending, error, recheck, locate, useAutomatic };
}
