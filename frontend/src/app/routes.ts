import {
  Film,
  LayoutDashboard,
  ScrollText,
  Settings,
  type LucideIcon,
} from "lucide-react";

/** The app has four screens and no URL bar, so a plain union beats a router. */
export type View = "dashboard" | "library" | "logs" | "settings";

export interface ViewMeta {
  id: View;
  label: string;
  icon: LucideIcon;
}

export const views: readonly ViewMeta[] = [
  { id: "dashboard", label: "Dashboard", icon: LayoutDashboard },
  { id: "library", label: "Library", icon: Film },
  { id: "logs", label: "Logs", icon: ScrollText },
  { id: "settings", label: "Settings", icon: Settings },
];
