import { FolderSync, LoaderCircle, RefreshCw } from "lucide-react";
import { GlassButton } from "@/shared/ui/glass";
import { useLibraryActions } from "../hooks/useLibraryActions";
import { useLibraryStore } from "../store";

/**
 * The library folder with its actions and any problem. A failed rescan keeps
 * the previous library, and says so.
 */
export function LibraryFolder({ root }: { root: string }) {
  const { choose, rescan, busy, error } = useLibraryActions();
  const scanning = useLibraryStore((s) => s.status?.scanning ?? false);
  const scanError = useLibraryStore((s) => s.status?.error ?? "");
  const watching = useLibraryStore((s) => s.status?.watching ?? false);
  // Go's own explanation of a failed scan is written for people; the command's
  // rejection is the raw error (with paths and system text), so it is only a
  // fallback for failures that never became a scan status.
  const problem = scanError || error;

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p
          className="min-w-0 flex-1 truncate text-sm text-muted-foreground select-text"
          title={root}
        >
          {root}
        </p>
        <div className="flex gap-2">
          <GlassButton size="sm" onClick={rescan} disabled={busy}>
            {scanning ? <LoaderCircle className="animate-spin" /> : <RefreshCw />}
            {scanning ? "Scanning…" : "Rescan"}
          </GlassButton>
          <GlassButton size="sm" onClick={choose} disabled={busy}>
            <FolderSync /> Change folder
          </GlassButton>
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {watching
          ? "Watching this folder: added and removed files appear by themselves."
          : "Changes on disk are not picked up automatically. Use Rescan after adding or removing files."}
      </p>
      {problem && (
        <p role="alert" className="text-sm text-destructive">
          {problem} Showing the last successful scan.
        </p>
      )}
    </div>
  );
}
