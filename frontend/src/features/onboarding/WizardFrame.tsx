import { useT } from "@/shared/i18n";
import { cn } from "@/shared/lib/cn";
import { stepNumber, wizardSteps, type WizardStep } from "./wizard";

interface WizardFrameProps {
  step: WizardStep;
  onSkip: () => void;
  children: React.ReactNode;
}

/** The progress line above each step, and the way out of the whole setup. */
export function WizardFrame({ step, onSkip, children }: WizardFrameProps) {
  const current = stepNumber(step);
  const { t } = useT();
  return (
    <div className="flex h-full flex-col gap-4">
      <nav aria-label={t("wizard.progress")} className="flex items-center justify-between gap-4 px-2 pt-1">
        <ol className="flex items-center gap-2">
          {wizardSteps.map((s, i) => (
            <li
              key={s.id}
              aria-current={s.id === step ? "step" : undefined}
              className={cn(
                "flex items-center gap-2 text-xs",
                s.id === step ? "font-medium text-foreground" : "text-muted-foreground",
              )}
            >
              <span
                className={cn(
                  "grid size-5 place-items-center rounded-full text-[11px]",
                  i + 1 <= current ? "bg-primary text-primary-foreground" : "bg-(--glass-surface-hover)",
                )}
              >
                {i + 1}
              </span>
              <span className="hidden sm:inline">{t(s.titleKey)}</span>
            </li>
          ))}
        </ol>
        <button
          type="button"
          onClick={onSkip}
          className="app-no-drag text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
        >
          {t("wizard.skip")}
        </button>
      </nav>
      <div className="min-h-0 flex-1">{children}</div>
    </div>
  );
}
