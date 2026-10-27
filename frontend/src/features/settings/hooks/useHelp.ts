import { useCallback, useState } from "react";
import { copyText, getDiagnostics, openHelpPage, type HelpTopic } from "@/shared/bridge";

interface Help {
  /** Copies the diagnostics to the clipboard; true when it worked. */
  copyDiagnostics: () => Promise<boolean>;
  open: (topic: HelpTopic) => Promise<void>;
  /** Why the last command failed, in Go's words. */
  error: string | null;
}

/** The commands behind the Help card. Nothing here is sent anywhere: the report goes to the clipboard. */
export function useHelp(): Help {
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(async <T>(command: () => Promise<T>): Promise<T | undefined> => {
    setError(null);
    try {
      return await command();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return undefined;
    }
  }, []);

  const copyDiagnostics = useCallback(async () => {
    const done = await run(async () => {
      await copyText(await getDiagnostics());
      return true;
    });
    return done === true;
  }, [run]);

  const open = useCallback(async (topic: HelpTopic) => void (await run(() => openHelpPage(topic))), [run]);

  return { copyDiagnostics, open, error };
}
