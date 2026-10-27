import { create } from "zustand";
import type { Profile } from "@/shared/bridge";

interface ProfilesStoreState {
  /** Null until Go reports. */
  profiles: Profile[] | null;
  /** A "profiles:changed" event arrived: always the newest truth. */
  apply: (profiles: Profile[]) => void;
  /** The startup query answered. An event that raced ahead of it wins. */
  hydrate: (profiles: Profile[]) => void;
}

/** Projection of Go's profiles; the editor works on a local copy, never on this. */
export const useProfilesStore = create<ProfilesStoreState>()((set) => ({
  profiles: null,
  apply: (profiles) => set({ profiles }),
  hydrate: (profiles) => set((state) => ({ profiles: state.profiles ?? profiles })),
}));
