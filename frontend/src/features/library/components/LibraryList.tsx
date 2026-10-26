import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { LibraryItem } from "@/shared/bridge";
import { LibraryRow, ROW_HEIGHT } from "./LibraryRow";

/** Virtualized, like the log list: a library of thousands costs a screenful of rows. */
export function LibraryList({
  items,
  emptyHint,
  libraryNames,
}: {
  items: LibraryItem[];
  emptyHint: string;
  /** Library ID → name, given when there is more than one library to tell apart. */
  libraryNames?: ReadonlyMap<string, string>;
}) {
  const scroller = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
  });

  if (items.length === 0) {
    return (
      <p className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {emptyHint}
      </p>
    );
  }

  return (
    <div ref={scroller} className="h-full overflow-auto py-1">
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((row) => {
          const item = items[row.index];
          if (!item) return null;
          return (
            <LibraryRow
              key={item.id}
              item={item}
              libraryName={libraryNames?.get(item.libraryId)}
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
