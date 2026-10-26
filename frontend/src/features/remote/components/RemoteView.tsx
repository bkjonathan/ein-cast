import { PinPanel } from "./PinPanel";
import { ProviderPanel } from "./ProviderPanel";

/** The Remote screen: how to connect from outside, and the PIN that guards it. */
export function RemoteView() {
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto">
      <ProviderPanel />
      <PinPanel />
    </div>
  );
}
