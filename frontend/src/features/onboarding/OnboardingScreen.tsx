import { FolderOpen, FolderX, LoaderCircle } from "lucide-react";
import { useLibraryActions, useLibraryStore } from "@/features/library";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";

/**
 * First-run screen: one decision, choose where the media lives. It is also what
 * you see when the folder from last time can no longer be opened, in which case
 * it says why.
 */
export function OnboardingScreen() {
  const { choose, busy, error } = useLibraryActions();
  const status = useLibraryStore((s) => s.status);

  const lastFolder = status?.error ? status.root : "";
  // The status text is written for people; the command's rejection is raw.
  const problem = status?.error || error;

  return (
    <div className="flex h-full items-center justify-center">
      <GlassPanel className="flex w-full max-w-md flex-col items-center gap-5 p-10 text-center">
        {lastFolder ? (
          <FolderX className="size-14 text-warning" strokeWidth={1.5} aria-hidden />
        ) : (
          <FolderOpen className="size-14 text-primary" strokeWidth={1.5} aria-hidden />
        )}
        <div className="space-y-1.5">
          <h2 className="text-xl font-semibold">
            {lastFolder ? "Can't open your library" : "Welcome to EinCast"}
          </h2>
          <p className="text-sm text-muted-foreground">
            {lastFolder
              ? "EinCast tried to reopen the folder you used last time."
              : "Pick the folder with your videos, music and photos. EinCast will scan it so your other screens can play it."}
          </p>
          {lastFolder && (
            <p className="truncate text-xs text-muted-foreground select-text" title={lastFolder}>
              {lastFolder}
            </p>
          )}
        </div>
        <GlassButton size="lg" onClick={choose} disabled={busy}>
          {busy ? <LoaderCircle className="animate-spin" /> : <FolderOpen />}
          {busy ? "Scanning…" : lastFolder ? "Choose another folder" : "Choose Media Folder"}
        </GlassButton>
        {problem && (
          <p role="alert" className="text-sm text-destructive">
            {problem}
          </p>
        )}
      </GlassPanel>
    </div>
  );
}
