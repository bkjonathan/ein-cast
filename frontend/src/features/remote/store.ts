import { create } from "zustand";
import type { RemoteProvider, RemoteStatus } from "@/shared/bridge";

interface RemoteStoreState {
  /** Null until Go reports; the UI shows no controls before then. */
  status: RemoteStatus | null;
  /** The providers this build offers. Fixed for a run, so fetched once. */
  providers: RemoteProvider[];
  /** A "remote:status" event arrived: always the newest truth. */
  applyStatus: (status: RemoteStatus) => void;
  /** The startup query answered. An event that raced ahead of it wins. */
  hydrate: (status: RemoteStatus) => void;
  setProviders: (providers: RemoteProvider[]) => void;
}

/** Projection of Go's remote access state; only events and startup queries write it. */
export const useRemoteStore = create<RemoteStoreState>()((set) => ({
  status: null,
  providers: [],
  applyStatus: (status) => set({ status }),
  hydrate: (status) => set((state) => ({ status: state.status ?? status })),
  setProviders: (providers) => set({ providers }),
}));
