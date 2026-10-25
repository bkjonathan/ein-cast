import type { LogAttr, LogLevel, LogLine } from "@/shared/bridge";
import { allLevels } from "./store";

/** `key=value key2=value2`, the compact form shown after the message. */
export function attrsText(attrs: readonly LogAttr[]): string {
  return attrs.map((a) => `${a.key}=${a.value}`).join(" ");
}

function matches(line: LogLine, needle: string): boolean {
  return (
    line.message.toLowerCase().includes(needle) ||
    line.source.toLowerCase().includes(needle) ||
    attrsText(line.attrs).toLowerCase().includes(needle)
  );
}

/**
 * Applies the level chips and the search box. Search is a case-insensitive
 * substring match over message, source and attributes. Returns the same array
 * when nothing is filtered, so memoized consumers do not re-render needlessly.
 */
export function filterLines(
  lines: LogLine[],
  levels: ReadonlySet<LogLevel>,
  query: string,
): LogLine[] {
  const needle = query.trim().toLowerCase();
  if (levels.size === allLevels.length && needle === "") return lines;
  return lines.filter((l) => levels.has(l.level) && (needle === "" || matches(l, needle)));
}

export function countByLevel(lines: readonly LogLine[]): Record<LogLevel, number> {
  const counts: Record<LogLevel, number> = { debug: 0, info: 0, warn: 0, error: 0 };
  for (const l of lines) counts[l.level]++;
  return counts;
}

const pad = (n: number, width = 2) => String(n).padStart(width, "0");

/** Local wall-clock time with milliseconds: 14:03:09.027. */
export function formatLogTime(ms: number): string {
  const d = new Date(ms);
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
}
