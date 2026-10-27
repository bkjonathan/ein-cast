import { Pause, Play, Search, Trash2 } from "lucide-react";
import type { LogLevel } from "@/shared/bridge";
import { useT } from "@/shared/i18n";
import { cn } from "@/shared/lib/cn";
import { Input } from "@/shared/ui/input";
import { GlassButton, GlassToolbar } from "@/shared/ui/glass";
import { allLevels, useLogStore } from "../store";

const dot: Record<LogLevel, string> = {
  debug: "bg-muted-foreground",
  info: "bg-primary",
  warn: "bg-warning",
  error: "bg-destructive",
};

interface LogToolbarProps {
  counts: Record<LogLevel, number>;
  shown: number;
  total: number;
}

/** Level chips, search, pause/resume and clear. */
export function LogToolbar({ counts, shown, total }: LogToolbarProps) {
  const levels = useLogStore((s) => s.levels);
  const query = useLogStore((s) => s.query);
  const paused = useLogStore((s) => s.paused);
  const toggleLevel = useLogStore((s) => s.toggleLevel);
  const setQuery = useLogStore((s) => s.setQuery);
  const setPaused = useLogStore((s) => s.setPaused);
  const clear = useLogStore((s) => s.clear);
  const { t, tn } = useT();

  return (
    <GlassToolbar className="flex-wrap">
      <div className="flex gap-1" role="group" aria-label={t("logs.levels")}>
        {allLevels.map((level) => {
          const on = levels.has(level);
          return (
            <button
              key={level}
              type="button"
              aria-pressed={on}
              onClick={() => toggleLevel(level)}
              className={cn(
                "app-no-drag flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition-colors",
                on
                  ? "bg-(--glass-surface-hover) text-foreground"
                  : "text-muted-foreground opacity-60 hover:opacity-100",
              )}
            >
              <span className={cn("size-2 rounded-full", dot[level])} aria-hidden />
              {level}
              <span className="tabular-nums text-muted-foreground">{counts[level]}</span>
            </button>
          );
        })}
      </div>

      <div className="relative min-w-40 flex-1">
        <Search
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden
        />
        <Input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t("logs.search")}
          aria-label={t("logs.search")}
          className="h-8 pl-8 text-sm"
        />
      </div>

      <span className="text-xs text-muted-foreground tabular-nums">
        {shown === total ? tn("logs.lines", total) : t("logs.shownOf", { shown, total })}
      </span>

      <GlassButton size="sm" onClick={() => setPaused(!paused)} aria-pressed={paused}>
        {paused ? <Play /> : <Pause />}
        {paused ? t("logs.resume") : t("logs.pause")}
      </GlassButton>
      <GlassButton size="sm" onClick={clear} disabled={total === 0}>
        <Trash2 /> {t("logs.clear")}
      </GlassButton>
    </GlassToolbar>
  );
}
