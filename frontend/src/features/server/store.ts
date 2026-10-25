import { create } from "zustand";
import type { ServerStatus } from "@/shared/bridge";

interface ServerStoreState {
  /** Null until Go reports; the UI shows no controls before then. */
  status: ServerStatus | null;
  /** A "server:state" event arrived: always the newest truth. */
  applyStatus: (status: ServerStatus) => void;
  /** The startup query answered. An event that raced ahead of it wins. */
  hydrate: (status: ServerStatus) => void;
}

/** Projection of Go's server lifecycle; only events and the startup query write it. */
export const useServerStore = create<ServerStoreState>()((set) => ({
  status: null,
  applyStatus: (status) => set({ status }),
  hydrate: (status) => set((state) => ({ status: state.status ?? status })),
}));
