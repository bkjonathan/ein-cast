import { create } from "zustand";
import type { LibraryStatus, LibrarySummary } from "@/shared/bridge";

interface LibraryState {
  /** Null until Go reports a scanned library. */
  summary: LibrarySummary | null;
  /** Null until Go reports. Afterwards: is a scan running, and why did the last one fail. */
  status: LibraryStatus | null;
  /** False until the first answer from Go, so the UI doesn't flash onboarding. */
  hydrated: boolean;
  /** A "library:scanned" event arrived: always the newest truth. */
  applyScanned: (summary: LibrarySummary) => void;
  /** A "library:status" event arrived: always the newest truth. */
  applyStatus: (status: LibraryStatus) => void;
  /** The startup queries answered. Events that raced ahead of them win. */
  hydrate: (summary: LibrarySummary | null, status: LibraryStatus) => void;
}

/** Projection of Go's library state; only events and the startup query write it. */
export const useLibraryStore = create<LibraryState>()((set) => ({
  summary: null,
  status: null,
  hydrated: false,
  applyScanned: (summary) => set({ summary }),
  applyStatus: (status) => set({ status }),
  hydrate: (summary, status) =>
    set((state) => ({
      summary: state.summary ?? summary,
      status: state.status ?? status,
      hydrated: true,
    })),
}));

/** The status to show while Go has not reported one. */
export const idleStatus: LibraryStatus = { scanning: false, root: "", error: "" };
