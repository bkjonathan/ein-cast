import { create } from "zustand";
import type { UpdateStatus } from "@/shared/bridge";

interface UpdatesStoreState {
  /** Null until Go reports. */
  status: UpdateStatus | null;
  /** An "update:status" event, or the answer to a check the person asked for: always the newest truth. */
  apply: (status: UpdateStatus) => void;
  /** The startup query answered. An event that raced ahead of it wins. */
  hydrate: (status: UpdateStatus) => void;
}

/** Projection of Go's update status. */
export const useUpdatesStore = create<UpdatesStoreState>()((set) => ({
  status: null,
  apply: (status) => set({ status }),
  hydrate: (status) => set((state) => ({ status: state.status ?? status })),
}));
