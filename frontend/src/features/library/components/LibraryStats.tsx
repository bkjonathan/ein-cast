import { Film, Image, Music, type LucideIcon } from "lucide-react";
import { formatBytes } from "@/shared/lib/format";
import { GlassPanel } from "@/shared/ui/glass";
import { useLibraryStore } from "../store";
import { LibraryFolder } from "./LibraryFolder";

const kinds: readonly {
  key: "video" | "audio" | "image";
  label: string;
  icon: LucideIcon;
}[] = [
  { key: "video", label: "Videos", icon: Film },
  { key: "audio", label: "Audio", icon: Music },
  { key: "image", label: "Images", icon: Image },
];

/** Counts by kind, total size and the scanned folder. */
export function LibraryStats() {
  const summary = useLibraryStore((s) => s.summary);
  if (!summary) return null;

  return (
    <GlassPanel className="flex flex-col gap-6 p-6">
      <header className="space-y-2">
        <h2 className="text-lg font-semibold">Library</h2>
        <LibraryFolder root={summary.root} />
      </header>

      {summary.total === 0 ? (
        <p className="text-sm text-muted-foreground">
          No supported media found in this folder.
        </p>
      ) : (
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          {kinds.map(({ key, label, icon: Icon }) => (
            <Stat key={key} label={label} icon={Icon} value={summary.counts[key]} />
          ))}
          <Stat label="Total size" value={formatBytes(summary.totalSize)} />
        </dl>
      )}
    </GlassPanel>
  );
}

function Stat({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: number | string;
  icon?: LucideIcon;
}) {
  return (
    <div className="glass rounded-glass-md p-4">
      <dt className="flex items-center gap-2 text-xs text-muted-foreground">
        {Icon && <Icon className="size-3.5" aria-hidden />}
        {label}
      </dt>
      <dd className="mt-1 text-2xl font-semibold tabular-nums">{value}</dd>
    </div>
  );
}
