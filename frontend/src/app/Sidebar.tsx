import { motion } from "motion/react";
import { useT } from "@/shared/i18n";
import { spring } from "@/shared/motion/presets";
import { cn } from "@/shared/lib/cn";
import { views, type View } from "./routes";

interface SidebarProps {
  active: View;
  onSelect: (view: View) => void;
  version?: string;
}

/** Primary navigation. The highlight glides between items via a shared layoutId. */
export function Sidebar({ active, onSelect, version }: SidebarProps) {
  const { t } = useT();
  return (
    <nav
      aria-label={t("nav.primary")}
      className="glass flex w-52 shrink-0 flex-col rounded-glass-lg p-2"
    >
      <ul className="flex flex-1 flex-col gap-1">
        {views.map(({ id, labelKey, icon: Icon }) => {
          const isActive = id === active;
          return (
            <li key={id}>
              <button
                type="button"
                aria-current={isActive ? "page" : undefined}
                onClick={() => onSelect(id)}
                className={cn(
                  "app-no-drag relative flex w-full items-center gap-3 rounded-glass-sm px-3 py-2 text-sm font-medium transition-colors",
                  isActive
                    ? "text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {isActive && (
                  <motion.span
                    layoutId="sidebar-active"
                    transition={spring.snappy}
                    className="absolute inset-0 rounded-glass-sm bg-(--glass-surface-hover)"
                  />
                )}
                <Icon className="relative size-4" aria-hidden />
                <span className="relative">{t(labelKey)}</span>
              </button>
            </li>
          );
        })}
      </ul>
      {version && (
        <p className="px-3 pt-2 text-xs text-muted-foreground">v{version}</p>
      )}
    </nav>
  );
}
