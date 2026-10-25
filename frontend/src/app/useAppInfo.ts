import { useEffect, useState } from "react";
import { getAppInfo, type AppInfo } from "@/shared/bridge";

/** Loads build info from Go once. Null until it arrives, or outside Wails. */
export function useAppInfo(): AppInfo | null {
  const [info, setInfo] = useState<AppInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    getAppInfo()
      .then((value) => {
        if (!cancelled) setInfo(value);
      })
      // Running in a plain browser (vite dev) has no Go runtime; the shell
      // simply renders without version info.
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  return info;
}
