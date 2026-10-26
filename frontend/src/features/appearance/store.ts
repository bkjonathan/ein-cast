import { create } from "zustand";
import { applyTheme, browserStorage, readTheme, saveTheme, type ThemeMode } from "./theme";

interface AppearanceState {
  mode: ThemeMode;
  /** Applies the theme to the page and remembers the choice. */
  setMode: (mode: ThemeMode) => void;
}

export const useAppearanceStore = create<AppearanceState>()((set) => ({
  mode: readTheme(browserStorage()),
  setMode: (mode) => {
    applyTheme(document.documentElement, mode);
    saveTheme(browserStorage(), mode);
    set({ mode });
  },
}));

/** Sets the page theme from storage. Call before the first render to avoid a flash. */
export function initTheme(): void {
  applyTheme(document.documentElement, useAppearanceStore.getState().mode);
}
