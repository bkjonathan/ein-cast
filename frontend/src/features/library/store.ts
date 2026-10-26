import { create } from "zustand";
import type { LibraryItem, LibraryState } from "@/shared/bridge";
import type { KindFilter, SortKey } from "./filter";

/** "all", or the ID of the one library to show. */
export type LibraryFilter = "all" | (string & {});

interface LibraryStoreState {
  /** Every library Go knows about, in the order they were added. */
  libraries: LibraryState[];
  /** False until the first answer from Go, so the UI doesn't flash onboarding. */
  hydrated: boolean;
  /** An event arrived, so Go's push is newer than any startup answer. */
  heard: boolean;
  /** Every file in every library, as of the last "library:scanned". */
  items: LibraryItem[];

  /** How the Library screen shows `items`: view state, not Go's. */
  kind: KindFilter;
  library: LibraryFilter;
  query: string;
  sort: SortKey;
  setKind: (kind: KindFilter) => void;
  setLibrary: (library: LibraryFilter) => void;
  setQuery: (query: string) => void;
  setSort: (sort: SortKey) => void;
  setItems: (items: LibraryItem[]) => void;

  /** A "library:scanned" or "library:status" event arrived: always the newest truth. */
  apply: (libraries: LibraryState[]) => void;
  /** The startup query answered. Events that raced ahead of it win. */
  hydrate: (libraries: LibraryState[]) => void;
}

/** Projection of Go's libraries; only events and the startup query write them. */
export const useLibraryStore = create<LibraryStoreState>()((set) => ({
  libraries: [],
  hydrated: false,
  heard: false,
  items: [],
  kind: "all",
  library: "all",
  query: "",
  sort: "name",
  setKind: (kind) => set({ kind }),
  setLibrary: (library) => set({ library }),
  setQuery: (query) => set({ query }),
  setSort: (sort) => set({ sort }),
  setItems: (items) => set({ items }),
  apply: (libraries) => set({ libraries, heard: true, hydrated: true }),
  hydrate: (libraries) =>
    set((state) => (state.heard ? { hydrated: true } : { libraries, hydrated: true })),
}));
