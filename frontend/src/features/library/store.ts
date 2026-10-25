import { create } from "zustand";
import type { LibrarySummary } from "@/shared/bridge";

interface LibraryState {
  /** Null until Go reports a scanned library. */
  summary: LibrarySummary | null;
  /** False until the first answer from Go, so the UI doesn't flash onboarding. */
  hydrated: boolean;
  /** A "library:scanned" event arrived: always the newest truth. */
  applyScanned: (summary: LibrarySummary) => void;
  /** The startup query answered. An event that raced ahead of it wins. */
  hydrate: (summary: LibrarySummary | null) => void;
}

/** Projection of Go's library state; only events and the startup query write it. */
export const useLibraryStore = create<LibraryState>()((set) => ({
  summary: null,
  hydrated: false,
  applyScanned: (summary) => set({ summary, hydrated: true }),
  hydrate: (summary) =>
    set((state) => ({ summary: state.summary ?? summary, hydrated: true })),
}));
