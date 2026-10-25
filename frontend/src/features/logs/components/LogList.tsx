import { useEffect, useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { LogLine } from "@/shared/bridge";
import { useLogStore } from "../store";
import { LogRow, ROW_HEIGHT } from "./LogRow";

/**
 * Virtualized list: only the rows in view exist in the DOM, so 5000 lines cost
 * about as much as 50. It follows new lines unless paused.
 */
export function LogList({ lines, emptyHint }: { lines: LogLine[]; emptyHint: string }) {
  const scroller = useRef<HTMLDivElement>(null);
  const paused = useLogStore((s) => s.paused);
  const setPaused = useLogStore((s) => s.setPaused);

  const virtualizer = useVirtualizer({
    count: lines.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 20,
  });

  useEffect(() => {
    if (!paused && lines.length > 0) {
      virtualizer.scrollToIndex(lines.length - 1, { align: "end" });
    }
  }, [lines.length, paused, virtualizer]);

  if (lines.length === 0) {
    return (
      <p className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {emptyHint}
      </p>
    );
  }

  return (
    <div
      ref={scroller}
      // Scrolling up is an explicit "let me read"; stop following. Resuming is
      // the toolbar button, so a stray scroll never flips it back.
      onWheel={(e) => {
        if (e.deltaY < 0 && !paused) setPaused(true);
      }}
      className="h-full overflow-auto py-1 select-text"
    >
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((row) => {
          const line = lines[row.index];
          if (!line) return null;
          return (
            <LogRow
              key={line.seq}
              line={line}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                height: row.size,
                transform: `translateY(${row.start}px)`,
              }}
            />
          );
        })}
      </div>
    </div>
  );
}
