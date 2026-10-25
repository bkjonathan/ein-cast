import { create } from "zustand";
import type { Settings } from "@/shared/bridge";

interface SettingsStoreState {
  /** Null until Go reports. */
  settings: Settings | null;
  /** A "settings:changed" event arrived: always the newest truth. */
  apply: (settings: Settings) => void;
  /** The startup query answered. An event that raced ahead of it wins. */
  hydrate: (settings: Settings) => void;
}

/** Projection of Go's saved settings; the form edits a local copy, never this. */
export const useSettingsStore = create<SettingsStoreState>()((set) => ({
  settings: null,
  apply: (settings) => set({ settings }),
  hydrate: (settings) => set((state) => ({ settings: state.settings ?? settings })),
}));
