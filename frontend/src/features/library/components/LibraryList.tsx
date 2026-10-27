import { useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { ListRow } from "../group";
import { LibraryRow, ROW_HEIGHT, SeasonRow, ShowRow } from "./LibraryRow";

const rowKey = (row: ListRow): string => {
  switch (row.type) {
    case "item":
    case "episode":
      return `${row.type}:${row.item.id}`;
    case "show":
    case "season":
      return `${row.type}:${row.key}`;
  }
};

/** Virtualized, like the log list: a library of thousands costs a screenful of rows. */
export function LibraryList({
  rows,
  emptyHint,
  libraryNames,
  onToggleShow,
}: {
  rows: ListRow[];
  emptyHint: string;
  /** Library ID → name, given when there is more than one library to tell apart. */
  libraryNames?: ReadonlyMap<string, string>;
  /** Opens or closes a series, by its key. */
  onToggleShow: (key: string) => void;
}) {
  const scroller = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
  });

  if (rows.length === 0) {
    return (
      <p className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {emptyHint}
      </p>
    );
  }

  return (
    <div ref={scroller} className="h-full overflow-auto py-1">
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((v) => {
          const row = rows[v.index];
          if (!row) return null;
          const style = {
            position: "absolute",
            top: 0,
            left: 0,
            width: "100%",
            height: v.size,
            transform: `translateY(${v.start}px)`,
          } as const;
          switch (row.type) {
            case "item":
              return <LibraryRow key={rowKey(row)} item={row.item} libraryName={libraryNames?.get(row.item.libraryId)} style={style} />;
            case "episode":
              return <LibraryRow key={rowKey(row)} item={row.item} style={style} indent />;
            case "season":
              return <SeasonRow key={rowKey(row)} season={row.season} style={style} />;
            case "show":
              return (
                <ShowRow
                  key={rowKey(row)}
                  name={row.name}
                  episodes={row.episodes}
                  seasons={row.seasons}
                  cover={row.cover}
                  open={row.open}
                  onToggle={() => onToggleShow(row.key)}
                  style={style}
                />
              );
          }
        })}
      </div>
    </div>
  );
}
