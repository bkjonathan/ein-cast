import { LoaderCircle } from "lucide-react";
import { GlassPanel } from "@/shared/ui/glass";

/** Shown while the first scan runs, including the automatic one at startup. */
export function ScanningScreen({ root }: { root: string }) {
  return (
    <div className="flex h-full items-center justify-center">
      <GlassPanel className="flex w-full max-w-md flex-col items-center gap-4 p-10 text-center">
        <LoaderCircle className="size-12 animate-spin text-primary" strokeWidth={1.5} aria-hidden />
        <h2 className="text-xl font-semibold">Scanning your library…</h2>
        {root && (
          <p className="max-w-full truncate text-sm text-muted-foreground" title={root}>
            {root}
          </p>
        )}
      </GlassPanel>
    </div>
  );
}
