import { AppearanceSettings } from "@/features/appearance";
import { LibraryManager } from "@/features/library";
import { AccessSettings } from "./AccessSettings";
import { FFmpegSettings } from "./FFmpegSettings";
import { NetworkSettings } from "./NetworkSettings";
import { PortSettings } from "./PortSettings";
import { ServerSettings } from "./ServerSettings";
import { StartupSettings } from "./StartupSettings";
import { TvSettings } from "./TvSettings";

/** The Settings screen: libraries, server, startup, port and network tools, access, Smart TVs, ffmpeg and appearance. */
export function SettingsView({ checkPort }: { checkPort?: number }) {
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto">
      <LibraryManager />
      <ServerSettings />
      <StartupSettings />
      <PortSettings checkPort={checkPort} />
      <NetworkSettings />
      <AccessSettings />
      <TvSettings />
      <FFmpegSettings />
      <AppearanceSettings />
    </div>
  );
}
