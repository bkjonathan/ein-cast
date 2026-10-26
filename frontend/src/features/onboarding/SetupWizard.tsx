import { useState } from "react";
import { isFirstScan, useLibraryStore } from "@/features/library";
import { ConnectStep } from "./ConnectStep";
import { OnboardingScreen } from "./OnboardingScreen";
import { ScanningScreen } from "./ScanningScreen";
import { ShareStep } from "./ShareStep";
import { WizardFrame } from "./WizardFrame";
import type { WizardStep } from "./wizard";

/**
 * First run, in three steps: pick the folder, start sharing, open it on a screen.
 * Step 1 is not a state of its own: it lasts until Go reports a library, so the
 * wizard cannot disagree with Go about whether a folder was added. Skipping, or
 * finishing, hands the window to the normal app.
 */
export function SetupWizard({ onDone }: { onDone: () => void }) {
  const libraries = useLibraryStore((s) => s.libraries);
  const [later, setLater] = useState<Exclude<WizardStep, "folder">>("share");

  const step: WizardStep = libraries.length === 0 || isFirstScan(libraries) ? "folder" : later;

  return (
    <WizardFrame step={step} onSkip={onDone}>
      {libraries.length === 0 ? (
        <OnboardingScreen />
      ) : isFirstScan(libraries) ? (
        <ScanningScreen root={libraries.find((l) => l.scanning)?.root ?? ""} />
      ) : later === "share" ? (
        <ShareStep onNext={() => setLater("connect")} />
      ) : (
        <ConnectStep onDone={onDone} />
      )}
    </WizardFrame>
  );
}
