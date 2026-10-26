import { useEffect } from "react";
import { setWindowTheme } from "@/shared/bridge";
import { useAppearanceStore } from "./store";

/**
 * Keeps the native window's chrome in step with the theme (it matters on
 * Windows only). Mount once, near the root. Runs at startup too, so a saved
 * choice reaches the window and not just the page.
 */
export function useWindowTheme(): void {
  const mode = useAppearanceStore((s) => s.mode);
  useEffect(() => {
    // No Go runtime (plain-browser dev) or an old build: the page theme is enough.
    setWindowTheme(mode).catch(() => undefined);
  }, [mode]);
}
