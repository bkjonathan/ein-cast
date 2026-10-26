import { FolderPlus, LoaderCircle, RefreshCw } from "lucide-react";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { useLibraryActions } from "../hooks/useLibraryActions";
import { useLibraryStore } from "../store";
import { LibraryCard } from "./LibraryCard";

/** The list of libraries with the controls to add one or scan them all. */
export function LibraryManager() {
  const libraries = useLibraryStore((s) => s.libraries);
  const { add, rescanAll, busy, error } = useLibraryActions();
  const scanning = libraries.some((l) => l.scanning);

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h2 className="text-lg font-semibold">Libraries</h2>
          <p className="text-sm text-muted-foreground">
            The folders EinCast serves. Add as many as you like; one cannot contain another.
          </p>
        </div>
        <div className="flex gap-2">
          {libraries.length > 1 && (
            <GlassButton size="sm" onClick={() => void rescanAll()} disabled={busy}>
              {scanning ? <LoaderCircle className="animate-spin" /> : <RefreshCw />} Rescan all
            </GlassButton>
          )}
          <GlassButton size="sm" onClick={() => void add()} disabled={busy}>
            <FolderPlus /> Add folder…
          </GlassButton>
        </div>
      </header>

      {libraries.map((l) => (
        <LibraryCard key={l.id} library={l} />
      ))}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
