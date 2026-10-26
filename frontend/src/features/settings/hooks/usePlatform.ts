import { useEffect, useState } from "react";
import { getAppInfo } from "@/shared/bridge";

/** "darwin", "windows" or "" until Go answers (and outside Wails, where it never does). */
export function usePlatform(): string {
  const [platform, setPlatform] = useState("");
  useEffect(() => {
    let cancelled = false;
    getAppInfo()
      .then((info) => {
        if (!cancelled) setPlatform(info.platform);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);
  return platform;
}
