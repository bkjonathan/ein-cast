import { FolderOpen, LoaderCircle } from "lucide-react";
import { useLibraryActions } from "@/features/library";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Logo } from "@/shared/ui/Logo";

/**
 * First-run screen: one decision, choose where the media lives. More folders
 * can be added later in Settings.
 */
export function OnboardingScreen() {
  const { add, busy, error } = useLibraryActions();
  const { t } = useT();

  return (
    <div className="flex h-full items-center justify-center">
      <GlassPanel className="flex w-full max-w-md flex-col items-center gap-5 p-10 text-center">
        <Logo className="size-20 drop-shadow-lg" />
        <div className="space-y-1.5">
          <h2 className="text-xl font-semibold">{t("onboarding.welcome")}</h2>
          <p className="text-sm text-muted-foreground">
            {t("onboarding.pick")}
          </p>
        </div>
        <GlassButton size="lg" onClick={() => void add()} disabled={busy}>
          {busy ? <LoaderCircle className="animate-spin" /> : <FolderOpen />}
          {busy ? t("onboarding.scanning") : t("onboarding.choose")}
        </GlassButton>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
      </GlassPanel>
    </div>
  );
}
