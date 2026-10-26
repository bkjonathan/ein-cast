import logo from "@/assets/logo.png";
import { cn } from "@/shared/lib/cn";

/** The EinCast app icon. Decorative: the name is always written next to it. */
export function Logo({ className }: { className?: string }) {
  return <img src={logo} alt="" draggable={false} className={cn("select-none", className)} />;
}
