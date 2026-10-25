import type { CSSProperties } from "react";
import type { LogLevel, LogLine } from "@/shared/bridge";
import { cn } from "@/shared/lib/cn";
import { attrsText, formatLogTime } from "../filter";

const levelStyle: Record<LogLevel, string> = {
  debug: "text-muted-foreground",
  info: "text-primary",
  warn: "text-warning",
  error: "text-destructive",
};

/** Height every row is laid out at; the virtualizer relies on it being fixed. */
export const ROW_HEIGHT = 24;

/** One single-line log record. The full text is in the tooltip and selectable. */
export function LogRow({ line, style }: { line: LogLine; style: CSSProperties }) {
  const attrs = attrsText(line.attrs);
  return (
    <div
      style={style}
      title={`${line.message} ${attrs}`.trim()}
      className={cn(
        "flex items-center gap-3 px-4 font-mono text-xs whitespace-nowrap",
        line.level === "error" && "bg-destructive/10",
      )}
    >
      <span className="shrink-0 text-muted-foreground tabular-nums">
        {formatLogTime(line.time)}
      </span>
      <span className={cn("w-10 shrink-0 font-semibold uppercase", levelStyle[line.level])}>
        {line.level}
      </span>
      <span className="w-14 shrink-0 truncate text-muted-foreground">{line.source}</span>
      <span className="min-w-0 truncate">{line.message}</span>
      {attrs && <span className="min-w-0 flex-1 truncate text-muted-foreground">{attrs}</span>}
    </div>
  );
}
