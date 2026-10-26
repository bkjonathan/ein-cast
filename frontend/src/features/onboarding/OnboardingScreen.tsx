import { FolderOpen, LoaderCircle } from "lucide-react";
import { useLibraryActions } from "@/features/library";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";

/**
 * First-run screen: one decision, choose where the media lives. More folders
 * can be added later in Settings.
 */
export function OnboardingScreen() {
  const { add, busy, error } = useLibraryActions();

  return (
    <div className="flex h-full items-center justify-center">
      <GlassPanel className="flex w-full max-w-md flex-col items-center gap-5 p-10 text-center">
        <FolderOpen className="size-14 text-primary" strokeWidth={1.5} aria-hidden />
        <div className="space-y-1.5">
          <h2 className="text-xl font-semibold">Welcome to EinCast</h2>
          <p className="text-sm text-muted-foreground">
            Pick the folder with your videos, music and photos. EinCast will scan it so your other screens can play it.
          </p>
        </div>
        <GlassButton size="lg" onClick={() => void add()} disabled={busy}>
          {busy ? <LoaderCircle className="animate-spin" /> : <FolderOpen />}
          {busy ? "Scanning…" : "Choose Media Folder"}
        </GlassButton>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
      </GlassPanel>
    </div>
  );
}
