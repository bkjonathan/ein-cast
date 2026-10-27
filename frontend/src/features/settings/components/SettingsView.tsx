import { AppearanceSettings, LanguageSettings } from "@/features/appearance";
import { LibraryManager } from "@/features/library";
import { ProfilesPanel } from "@/features/profiles";
import { AccessSettings } from "./AccessSettings";
import { FFmpegSettings } from "./FFmpegSettings";
import { NetworkSettings } from "./NetworkSettings";
import { PortSettings } from "./PortSettings";
import { ServerSettings } from "./ServerSettings";
import { StartupSettings } from "./StartupSettings";
import { TvSettings } from "./TvSettings";
import { UpdateSettings } from "./UpdateSettings";

/** The Settings screen: libraries, server, startup, port and network tools, access, Smart TVs, ffmpeg, appearance, language and updates. */
export function SettingsView({ checkPort, focusUpdates }: { checkPort?: number; focusUpdates?: number }) {
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto">
      <LibraryManager />
      <ProfilesPanel />
      <ServerSettings />
      <StartupSettings />
      <PortSettings checkPort={checkPort} />
      <NetworkSettings />
      <AccessSettings />
      <TvSettings />
      <FFmpegSettings />
      <AppearanceSettings />
      <LanguageSettings />
      <UpdateSettings focus={focusUpdates} />
    </div>
  );
}
