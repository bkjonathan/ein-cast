import { LibraryFolder, useLibraryStore } from "@/features/library";
import { GlassPanel } from "@/shared/ui/glass";
import { AccessSettings } from "./AccessSettings";
import { ServerSettings } from "./ServerSettings";

/** The Settings screen: server, access, and library folder. */
export function SettingsView() {
  const root = useLibraryStore((s) => s.summary?.root);

  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto">
      <ServerSettings />
      <AccessSettings />
      {root && (
        <GlassPanel className="flex flex-col gap-3 p-6">
          <h2 className="text-lg font-semibold">Library folder</h2>
          <LibraryFolder root={root} />
        </GlassPanel>
      )}
    </div>
  );
}
