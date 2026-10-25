import { ConnectCard } from "@/features/connect";
import { LibraryStats } from "@/features/library";
import { ServerPanel } from "@/features/server";

/** Home screen: server controls, how to connect, and what is being served. */
export function DashboardView() {
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto">
      <ServerPanel />
      <ConnectCard />
      <LibraryStats />
    </div>
  );
}
