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
  /** Phase that delivers the real screen; shown by the placeholder. */
  phase: string;
}

export const views: readonly ViewMeta[] = [
  { id: "dashboard", label: "Dashboard", icon: LayoutDashboard, phase: "Phase 3" },
  { id: "library", label: "Library", icon: Film, phase: "Phase 2" },
  { id: "logs", label: "Logs", icon: ScrollText, phase: "Phase 4" },
  { id: "settings", label: "Settings", icon: Settings, phase: "Phase 6" },
];
