import { useState } from "react";
import { Sidebar } from "./Sidebar";
import { TitleBar } from "./TitleBar";
import { ViewPlaceholder } from "./ViewPlaceholder";
import { views, type View } from "./routes";
import { useAppInfo } from "./useAppInfo";

/** Layout shell: title bar on top, sidebar on the left, active view in the middle. */
export function App() {
  const [active, setActive] = useState<View>("dashboard");
  const info = useAppInfo();
  const current = views.find((v) => v.id === active) ?? views[0];

  return (
    <div className="flex h-full flex-col">
      <TitleBar name={info?.name ?? "EinCast"} />
      <div className="flex min-h-0 flex-1 gap-3 px-3 pb-3">
        <Sidebar active={active} onSelect={setActive} version={info?.version} />
        <main className="min-w-0 flex-1">
          {current && <ViewPlaceholder key={current.id} {...current} />}
        </main>
      </div>
    </div>
  );
}
