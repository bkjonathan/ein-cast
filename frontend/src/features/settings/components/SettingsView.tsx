import { AppearanceSettings } from "@/features/appearance";
import { LibraryManager } from "@/features/library";
import { AccessSettings } from "./AccessSettings";
import { ServerSettings } from "./ServerSettings";
import { TvSettings } from "./TvSettings";

/** The Settings screen: libraries, server, access, Smart TVs and appearance. */
export function SettingsView() {
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto">
      <LibraryManager />
      <ServerSettings />
      <AccessSettings />
      <TvSettings />
      <AppearanceSettings />
    </div>
  );
}
