import { useCallback, useEffect, useRef, useState } from "react";
import { copyText } from "@/shared/bridge";

/** Copy-to-clipboard that remembers what was copied for a moment, to show "Copied". */
export function useCopy(resetMs = 1500): {
  copied: string | null;
  copy: (text: string) => Promise<void>;
} {
  const [copied, setCopied] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(timer.current), []);

  const copy = useCallback(
    async (text: string) => {
      clearTimeout(timer.current);
      try {
        await copyText(text);
      } catch {
        // The address is selectable on screen, so a failed copy is not fatal.
        setCopied(null);
        return;
      }
      setCopied(text);
      timer.current = setTimeout(() => setCopied(null), resetMs);
    },
    [resetMs],
  );

  return { copied, copy };
}
