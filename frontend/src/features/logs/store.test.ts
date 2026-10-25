import { beforeEach, describe, expect, it } from "vitest";
import type { LogLine } from "@/shared/bridge";
import { allLevels, MAX_LINES, useLogStore } from "./store";

const line = (seq: number, level: LogLine["level"] = "info"): LogLine => ({
  seq,
  time: seq * 1000,
  level,
  source: "app",
  message: `m${seq}`,
  attrs: [],
});
const range = (from: number, to: number) =>
  Array.from({ length: to - from + 1 }, (_, i) => line(from + i));
const seqs = () => useLogStore.getState().lines.map((l) => l.seq);

beforeEach(() =>
  useLogStore.setState({
    lines: [],
    levels: new Set(allLevels),
    query: "",
    paused: false,
    floor: 0,
  }),
);

describe("append", () => {
  it("adds new lines in order", () => {
    useLogStore.getState().append(range(1, 3));
    useLogStore.getState().append(range(4, 5));
    expect(seqs()).toEqual([1, 2, 3, 4, 5]);
  });

  it("ignores lines it has already seen", () => {
    useLogStore.getState().append(range(1, 5));
    useLogStore.getState().append(range(3, 7));
    expect(seqs()).toEqual([1, 2, 3, 4, 5, 6, 7]);
  });

  it("keeps the state object when there is nothing new", () => {
    useLogStore.getState().append(range(1, 3));
    const before = useLogStore.getState().lines;
    useLogStore.getState().append(range(1, 3));
    expect(useLogStore.getState().lines).toBe(before);
  });

  it("caps memory by dropping the oldest lines", () => {
    useLogStore.getState().append(range(1, MAX_LINES + 100));
    expect(useLogStore.getState().lines).toHaveLength(MAX_LINES);
    expect(seqs()[0]).toBe(101);
    expect(seqs().at(-1)).toBe(MAX_LINES + 100);
  });
});

describe("hydrate", () => {
  it("fills an empty store with the history", () => {
    useLogStore.getState().hydrate(range(1, 4));
    expect(seqs()).toEqual([1, 2, 3, 4]);
  });

  it("joins history and live lines that overlap, without gaps or duplicates", () => {
    useLogStore.getState().append(range(10, 12)); // live events won the race
    useLogStore.getState().hydrate(range(1, 10)); // history ends at 10, overlapping
    expect(seqs()).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]);
  });

  it("never replaces lines it already has", () => {
    useLogStore.getState().append([{ ...line(5), message: "live version" }]);
    useLogStore.getState().hydrate(range(1, 8));
    const five = useLogStore.getState().lines.find((l) => l.seq === 5);
    expect(five?.message).toBe("live version");
    expect(seqs()).toEqual([1, 2, 3, 4, 5]);
  });
});

describe("clear", () => {
  it("empties the view", () => {
    useLogStore.getState().append(range(1, 5));
    useLogStore.getState().clear();
    expect(seqs()).toEqual([]);
  });

  it("does not let cleared lines come back from a delayed batch or the history", () => {
    useLogStore.getState().append(range(1, 5));
    useLogStore.getState().clear();

    useLogStore.getState().append(range(3, 7)); // a late batch overlapping the cleared range
    expect(seqs()).toEqual([6, 7]);

    useLogStore.getState().hydrate(range(1, 7)); // history arriving after the clear
    expect(seqs()).toEqual([6, 7]);
  });

  it("clearing an already empty view keeps the floor", () => {
    useLogStore.getState().append(range(1, 3));
    useLogStore.getState().clear();
    useLogStore.getState().clear();
    useLogStore.getState().append(range(1, 4));
    expect(seqs()).toEqual([4]);
  });
});

describe("view state", () => {
  it("toggles a level on and off without mutating the old set", () => {
    const before = useLogStore.getState().levels;
    useLogStore.getState().toggleLevel("debug");
    expect(useLogStore.getState().levels.has("debug")).toBe(false);
    expect(before.has("debug")).toBe(true); // the previous Set object is untouched
    useLogStore.getState().toggleLevel("debug");
    expect(useLogStore.getState().levels.has("debug")).toBe(true);
  });

  it("stores the query and pause flag", () => {
    useLogStore.getState().setQuery("http");
    useLogStore.getState().setPaused(true);
    expect(useLogStore.getState()).toMatchObject({ query: "http", paused: true });
  });
});
