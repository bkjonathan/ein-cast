import { useMemo } from "react";
import { useT } from "@/shared/i18n";
import { GlassPanel } from "@/shared/ui/glass";
import { countByLevel, filterLines } from "../filter";
import { useLogStore } from "../store";
import { LogList } from "./LogList";
import { LogToolbar } from "./LogToolbar";

/** The Logs screen: filters on top, the virtualized list below. */
export function LogsView() {
  const lines = useLogStore((s) => s.lines);
  const levels = useLogStore((s) => s.levels);
  const query = useLogStore((s) => s.query);
  const { t } = useT();

  const visible = useMemo(() => filterLines(lines, levels, query), [lines, levels, query]);
  const counts = useMemo(() => countByLevel(lines), [lines]);

  return (
    <div className="flex h-full flex-col gap-3">
      <LogToolbar counts={counts} shown={visible.length} total={lines.length} />
      <GlassPanel className="min-h-0 flex-1 overflow-hidden">
        <LogList
          lines={visible}
          emptyHint={lines.length === 0 ? t("logs.waiting") : t("logs.noMatch")}
        />
      </GlassPanel>
    </div>
  );
}
