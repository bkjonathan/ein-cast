import { Film, FolderX, Image, LoaderCircle, Music, type LucideIcon } from "lucide-react";
import { useT, type MessageKey } from "@/shared/i18n";
import { formatBytes } from "@/shared/lib/format";
import { GlassPanel } from "@/shared/ui/glass";
import { totalsOf } from "../derive";
import { useLibraryStore } from "../store";

const kinds: readonly {
  key: "video" | "audio" | "image";
  labelKey: MessageKey;
  icon: LucideIcon;
}[] = [
  { key: "video", labelKey: "stats.videos", icon: Film },
  { key: "audio", labelKey: "stats.audio", icon: Music },
  { key: "image", labelKey: "stats.images", icon: Image },
];

/** Counts by kind and total size across every library, and how each library is doing. */
export function LibraryStats() {
  const libraries = useLibraryStore((s) => s.libraries);
  const { t, tn } = useT();
  if (libraries.length === 0) return null;
  const totals = totalsOf(libraries);

  return (
    <GlassPanel className="flex flex-col gap-6 p-6">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold">{t("stats.title")}</h2>
        <p className="text-sm text-muted-foreground">
          {t("stats.manage", { folders: tn("stats.folders", libraries.length) })}
        </p>
      </header>

      {totals.total === 0 ? (
        <p className="text-sm text-muted-foreground">{t("stats.none")}</p>
      ) : (
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          {kinds.map(({ key, labelKey, icon: Icon }) => (
            <Stat key={key} label={t(labelKey)} icon={Icon} value={totals.counts[key]} />
          ))}
          <Stat label={t("stats.totalSize")} value={formatBytes(totals.totalSize)} />
        </dl>
      )}

      {libraries.length > 1 || libraries.some((l) => l.error) ? (
        <ul className="flex flex-col gap-2" aria-label={t("stats.libraries")}>
          {libraries.map((l) => (
            <li key={l.id} className="flex min-w-0 items-center gap-3 text-sm">
              {l.scanning ? (
                <LoaderCircle className="size-4 shrink-0 animate-spin text-muted-foreground" aria-hidden />
              ) : l.error ? (
                <FolderX className="size-4 shrink-0 text-destructive" aria-hidden />
              ) : (
                <span className="size-4 shrink-0" aria-hidden />
              )}
              <span className="min-w-0 flex-1 truncate" title={l.root}>
                {l.name}
              </span>
              <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                {l.error ? l.error : tn("stats.files", l.total)}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
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
