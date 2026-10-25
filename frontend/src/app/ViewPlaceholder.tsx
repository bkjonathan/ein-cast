import { GlassPanel } from "@/shared/ui/glass";
import type { ViewMeta } from "./routes";

/** Stand-in content until a view's feature slice lands. */
export function ViewPlaceholder({ label, icon: Icon, phase }: ViewMeta) {
  return (
    <GlassPanel className="flex h-full flex-col items-center justify-center gap-3 text-center">
      <Icon className="size-10 text-muted-foreground" aria-hidden />
      <h2 className="text-lg font-semibold">{label}</h2>
      <p className="text-sm text-muted-foreground">Arrives in {phase}.</p>
    </GlassPanel>
  );
}
