import { LibraryStats } from "@/features/library";
import { ServerPanel } from "@/features/server";

/** Home screen: server controls on top, what the server is serving below. */
export function DashboardView() {
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto">
      <ServerPanel />
      <LibraryStats />
    </div>
  );
}
