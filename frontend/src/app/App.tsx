import { useEffect, useState } from "react";
import { useWindowTheme } from "@/features/appearance";
import { isFirstScan, LibraryView, useLibraryStore, useLibrarySync } from "@/features/library";
import { LogsView, useLogSync } from "@/features/logs";
import { useProfilesSync } from "@/features/profiles";
import { OnboardingScreen, ScanningScreen, SetupWizard } from "@/features/onboarding";
import { RemoteView, useRemoteSync } from "@/features/remote";
import { useServerSync } from "@/features/server";
import { useUpdatesSync } from "@/features/updates";
import { SettingsView, useSettingsSync } from "@/features/settings";
import { DashboardView } from "./DashboardView";
import { Sidebar } from "./Sidebar";
import { TitleBar } from "./TitleBar";
import type { View } from "./routes";
import { useAppInfo } from "./useAppInfo";

/** Layout shell: title bar on top, sidebar on the left, active view in the middle. */
export function App() {
  const [active, setActive] = useState<View>("dashboard");
  // Set when the Dashboard sends the person to the port tool; cleared when they
  // leave Settings, so opening Settings later does not search again by itself.
  const [portToCheck, setPortToCheck] = useState<number | undefined>(undefined);
  // Counts the times the sidebar's update notice sent the person to the updates
  // card (a counter, so a second click scrolls again); 0 once they leave Settings.
  const [focusUpdates, setFocusUpdates] = useState(0);
  const select = (view: View, options?: { updates?: boolean }) => {
    if (view !== "settings") setPortToCheck(undefined);
    setFocusUpdates((n) => (view === "settings" && options?.updates ? n + 1 : 0));
    setActive(view);
  };
  const info = useAppInfo();
  useLibrarySync();
  useServerSync();
  useLogSync();
  useSettingsSync();
  useRemoteSync();
  useProfilesSync();
  useUpdatesSync();
  useWindowTheme();

  const hydrated = useLibraryStore((s) => s.hydrated);
  const libraries = useLibraryStore((s) => s.libraries);

  // The first-run wizard runs only for a person who starts with no library. It is
  // decided once, when Go first reports, so adding the first folder does not
  // make an existing install look new, and finishing it cannot bring it back.
  const [setup, setSetup] = useState<"undecided" | "active" | "done">("undecided");
  useEffect(() => {
    if (hydrated && setup === "undecided") setSetup(libraries.length === 0 ? "active" : "done");
  }, [hydrated, setup, libraries.length]);

  return (
    <div className="flex h-full flex-col">
      <TitleBar name={info?.name ?? "EinCast"} onOpenRemote={() => setActive("remote")} />
      <div className="flex min-h-0 flex-1 gap-3 px-3 pb-3">
        {!hydrated || setup === "undecided" ? null : setup === "active" ? (
          <div className="min-w-0 flex-1">
            <SetupWizard onDone={() => setSetup("done")} />
          </div>
        ) : libraries.length === 0 || isFirstScan(libraries) ? (
          // Nothing to show yet: the first run, or the very first scan.
          <div className="min-w-0 flex-1">
            {libraries.length === 0 ? (
              <OnboardingScreen />
            ) : (
              <ScanningScreen root={libraries.find((l) => l.scanning)?.root ?? ""} />
            )}
          </div>
        ) : (
          <>
            <Sidebar active={active} onSelect={select} version={info?.version} />
            <main className="min-w-0 flex-1">
              <ActiveView
                view={active}
                portToCheck={portToCheck}
                focusUpdates={focusUpdates}
                onFreePort={(port) => {
                  setPortToCheck(port);
                  setActive("settings");
                }}
              />
            </main>
          </>
        )}
      </div>
    </div>
  );
}

interface ActiveViewProps {
  view: View;
  portToCheck: number | undefined;
  focusUpdates: number;
  onFreePort: (port: number) => void;
}

function ActiveView({ view, portToCheck, focusUpdates, onFreePort }: ActiveViewProps) {
  switch (view) {
    case "dashboard":
      return <DashboardView onFreePort={onFreePort} />;
    case "library":
      return <LibraryView />;
    case "remote":
      return <RemoteView />;
    case "logs":
      return <LogsView />;
    case "settings":
      return <SettingsView checkPort={portToCheck} focusUpdates={focusUpdates} />;
  }
}
