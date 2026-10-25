import { useState } from "react";
import { LibraryStats, useLibraryStore, useLibrarySync } from "@/features/library";
import { LogsView, useLogSync } from "@/features/logs";
import { OnboardingScreen } from "@/features/onboarding";
import { useServerSync } from "@/features/server";
import { DashboardView } from "./DashboardView";
import { Sidebar } from "./Sidebar";
import { TitleBar } from "./TitleBar";
import { ViewPlaceholder } from "./ViewPlaceholder";
import { views, type View } from "./routes";
import { useAppInfo } from "./useAppInfo";

/** Layout shell: title bar on top, sidebar on the left, active view in the middle. */
export function App() {
  const [active, setActive] = useState<View>("dashboard");
  const info = useAppInfo();
  useLibrarySync();
  useServerSync();
  useLogSync();
  const hydrated = useLibraryStore((s) => s.hydrated);
  const hasLibrary = useLibraryStore((s) => s.summary !== null);

  return (
    <div className="flex h-full flex-col">
      <TitleBar name={info?.name ?? "EinCast"} />
      <div className="flex min-h-0 flex-1 gap-3 px-3 pb-3">
        {!hydrated ? null : !hasLibrary ? (
          <div className="min-w-0 flex-1">
            <OnboardingScreen />
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
  if (view === "dashboard") return <DashboardView />;
  if (view === "library") return <LibraryStats />;
  if (view === "logs") return <LogsView />;
  const meta = views.find((v) => v.id === view);
  return meta ? <ViewPlaceholder key={meta.id} {...meta} /> : null;
}
