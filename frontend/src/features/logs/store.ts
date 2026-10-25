import { create } from "zustand";
import type { LogLevel, LogLine } from "@/shared/bridge";

/** Lines kept in memory. Older ones fall off the front. */
export const MAX_LINES = 5000;

export const allLevels: readonly LogLevel[] = ["debug", "info", "warn", "error"];

interface LogsState {
  lines: LogLine[];
  /** Levels currently shown. */
  levels: ReadonlySet<LogLevel>;
  query: string;
  /** When true the list stays where it is instead of following new lines. */
  paused: boolean;
  /**
   * Highest seq the user cleared. Anything at or below it is never shown again,
   * even if a delayed batch or the startup history arrives afterwards.
   */
  floor: number;

  /** Live lines from "log:line" events. Lines already seen are ignored. */
  append: (incoming: LogLine[]) => void;
  /** The startup history. Only fills in what is older than what we have. */
  hydrate: (history: LogLine[]) => void;
  clear: () => void;
  toggleLevel: (level: LogLevel) => void;
  setQuery: (query: string) => void;
  setPaused: (paused: boolean) => void;
}

function trim(lines: LogLine[]): LogLine[] {
  return lines.length > MAX_LINES ? lines.slice(-MAX_LINES) : lines;
}

/** Projection of Go's log; view state (filters, pause) lives beside it. */
export const useLogStore = create<LogsState>()((set) => ({
  lines: [],
  levels: new Set(allLevels),
  query: "",
  paused: false,
  floor: 0,

  append: (incoming) =>
    set((state) => {
      const newest = Math.max(state.lines.at(-1)?.seq ?? 0, state.floor);
      const fresh = incoming.filter((l) => l.seq > newest);
      if (fresh.length === 0) return state;
      return { lines: trim([...state.lines, ...fresh]) };
    }),

  hydrate: (history) =>
    set((state) => {
      const oldest = state.lines[0]?.seq ?? Infinity;
      const older = history.filter((l) => l.seq < oldest && l.seq > state.floor);
      if (older.length === 0) return state;
      return { lines: trim([...older, ...state.lines]) };
    }),

  clear: () =>
    set((state) => ({
      lines: [],
      floor: Math.max(state.lines.at(-1)?.seq ?? 0, state.floor),
    })),

  toggleLevel: (level) =>
    set((state) => {
      const levels = new Set(state.levels);
      if (!levels.delete(level)) levels.add(level);
      return { levels };
    }),

  setQuery: (query) => set({ query }),
  setPaused: (paused) => set({ paused }),
}));
