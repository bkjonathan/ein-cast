import type { MessageKey } from "@/shared/i18n";
import {
  Film,
  Globe,
  LayoutDashboard,
  ScrollText,
  Settings,
  type LucideIcon,
} from "lucide-react";

/** The app has five screens and no URL bar, so a plain union beats a router. */
export type View = "dashboard" | "library" | "remote" | "logs" | "settings";

export interface ViewMeta {
  id: View;
  labelKey: MessageKey;
  icon: LucideIcon;
}

export const views: readonly ViewMeta[] = [
  { id: "dashboard", labelKey: "nav.dashboard", icon: LayoutDashboard },
  { id: "library", labelKey: "nav.library", icon: Film },
  { id: "remote", labelKey: "nav.remote", icon: Globe },
  { id: "logs", labelKey: "nav.logs", icon: ScrollText },
  { id: "settings", labelKey: "nav.settings", icon: Settings },
];
