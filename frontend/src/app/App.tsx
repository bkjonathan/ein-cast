import { useState } from "react";
import { useWindowTheme } from "@/features/appearance";
import { LibraryView, useLibraryStore, useLibrarySync } from "@/features/library";
import { LogsView, useLogSync } from "@/features/logs";
import { OnboardingScreen, ScanningScreen } from "@/features/onboarding";
import { RemoteView, useRemoteSync } from "@/features/remote";
import { useServerSync } from "@/features/server";
import { SettingsView, useSettingsSync } from "@/features/settings";
import { DashboardView } from "./DashboardView";
import { Sidebar } from "./Sidebar";
import { TitleBar } from "./TitleBar";
import type { View } from "./routes";
import { useAppInfo } from "./useAppInfo";

/** Layout shell: title bar on top, sidebar on the left, active view in the middle. */
export function App() {
  const [active, setActive] = useState<View>("dashboard");
  const info = useAppInfo();
  useLibrarySync();
  useServerSync();
  useLogSync();
  useSettingsSync();
  useRemoteSync();
  useWindowTheme();

  const hydrated = useLibraryStore((s) => s.hydrated);
  const hasLibrary = useLibraryStore((s) => s.summary !== null);
  const status = useLibraryStore((s) => s.status);

  return (
    <div className="flex h-full flex-col">
      <TitleBar name={info?.name ?? "EinCast"} onOpenRemote={() => setActive("remote")} />
      <div className="flex min-h-0 flex-1 gap-3 px-3 pb-3">
        {!hydrated ? null : !hasLibrary ? (
          // No library yet: either the first run, or the last folder is still
          // being (re)opened, or it could not be opened.
          <div className="min-w-0 flex-1">
            {status?.scanning ? <ScanningScreen root={status.root} /> : <OnboardingScreen />}
          </div>
        ) : (
          <>
            <Sidebar active={active} onSelect={setActive} version={info?.version} />
            <main className="min-w-0 flex-1">
              <ActiveView view={active} />
            </main>
          </>
        )}
      </div>
    </div>
  );
}

function ActiveView({ view }: { view: View }) {
  switch (view) {
    case "dashboard":
      return <DashboardView />;
    case "library":
      return <LibraryView />;
    case "remote":
      return <RemoteView />;
    case "logs":
      return <LogsView />;
    case "settings":
      return <SettingsView />;
  }
}
