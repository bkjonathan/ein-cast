import { describe, expect, it } from "vitest";
import type { LogLevel, LogLine } from "@/shared/bridge";
import { attrsText, countByLevel, filterLines, formatLogTime } from "./filter";

const line = (over: Partial<LogLine>): LogLine => ({
  seq: 1,
  time: 0,
  level: "info",
  source: "app",
  message: "hello",
  attrs: [],
  ...over,
});

const all = new Set<LogLevel>(["debug", "info", "warn", "error"]);

const lines = [
  line({ seq: 1, level: "info", source: "http", message: "request", attrs: [{ key: "path", value: "/stream/AB12" }] }),
  line({ seq: 2, level: "error", source: "server", message: "Server died", attrs: [{ key: "error", value: "address in use" }] }),
  line({ seq: 3, level: "debug", source: "library", message: "walking" }),
  line({ seq: 4, level: "warn", source: "library", message: "skipping unreadable entry" }),
];
const seqsOf = (ls: LogLine[]) => ls.map((l) => l.seq);

describe("filterLines", () => {
  it("returns the very same array when nothing is filtered", () => {
    expect(filterLines(lines, all, "")).toBe(lines);
    expect(filterLines(lines, all, "   ")).toBe(lines);
  });

  it("filters by level", () => {
    const levels = new Set<LogLevel>(["warn", "error"]);
    expect(seqsOf(filterLines(lines, levels, ""))).toEqual([2, 4]);
  });

  it("shows nothing when every level is off", () => {
    expect(filterLines(lines, new Set(), "")).toEqual([]);
  });

  it.each([
    ["message, case-insensitively", "SERVER DIED", [2]],
    ["source", "library", [3, 4]],
    ["attribute value", "stream/ab12", [1]],
    ["attribute as key=value", "error=address", [2]],
    ["surrounding spaces ignored", "  walking  ", [3]],
    ["no match", "zzz", []],
  ])("search matches %s", (_name, query, expected) => {
    expect(seqsOf(filterLines(lines, all, query))).toEqual(expected);
  });

  it("combines level and search", () => {
    const levels = new Set<LogLevel>(["warn"]);
    expect(seqsOf(filterLines(lines, levels, "library"))).toEqual([4]);
  });
});

describe("countByLevel", () => {
  it("counts every level, including zeros", () => {
    expect(countByLevel(lines)).toEqual({ debug: 1, info: 1, warn: 1, error: 1 });
    expect(countByLevel([])).toEqual({ debug: 0, info: 0, warn: 0, error: 0 });
  });
});

describe("attrsText", () => {
  it("joins key=value pairs", () => {
    expect(attrsText([{ key: "a", value: "1" }, { key: "b", value: "x y" }])).toBe("a=1 b=x y");
    expect(attrsText([])).toBe("");
  });
});

describe("formatLogTime", () => {
  it("formats local time with zero-padded milliseconds", () => {
    expect(formatLogTime(new Date(2024, 0, 2, 3, 4, 5, 7).getTime())).toBe("03:04:05.007");
    expect(formatLogTime(new Date(2024, 5, 30, 23, 59, 59, 999).getTime())).toBe("23:59:59.999");
  });
});
