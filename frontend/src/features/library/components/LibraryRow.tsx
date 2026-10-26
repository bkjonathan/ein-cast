import { Film, Image, Music, type LucideIcon } from "lucide-react";
import type { CSSProperties } from "react";
import type { LibraryItem, MediaKind } from "@/shared/bridge";
import { formatBytes } from "@/shared/lib/format";
import { useThumbnail } from "../hooks/useThumbnail";

const icons: Record<MediaKind, LucideIcon> = { video: Film, audio: Music, image: Image };

/** Height every row is laid out at; the virtualizer relies on it being fixed. */
export const ROW_HEIGHT = 52;

/** Folder part of a slash path with its trailing slash, or "" at the top level. */
function folderOf(relPath: string): string {
  const cut = relPath.lastIndexOf("/");
  return cut < 0 ? "" : relPath.slice(0, cut);
}

/** One file: icon, name, the folder it lives in (with its library, when there are several), and its size. */
export function LibraryRow({
  item,
  style,
  libraryName,
}: {
  item: LibraryItem;
  style: CSSProperties;
  libraryName?: string | undefined;
}) {
  const Icon = icons[item.kind];
  const folder = [libraryName, folderOf(item.relPath)].filter(Boolean).join(" · ");
  const picture = useThumbnail(item);
  return (
    <div style={style} title={item.relPath} className="flex items-center gap-3 px-4">
      <span className="glass flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-glass-sm">
        {picture ? (
          <img src={picture} alt="" className="size-full object-cover" draggable={false} />
        ) : (
          <Icon className="size-4 text-muted-foreground" aria-hidden />
        )}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">{item.name}</p>
        {folder && <p className="truncate text-xs text-muted-foreground">{folder}</p>}
      </div>
      <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
        {formatBytes(item.size)}
      </span>
    </div>
  );
}
