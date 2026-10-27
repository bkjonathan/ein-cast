import { Check, Copy } from "lucide-react";
import { useT } from "@/shared/i18n";
import { GlassButton } from "@/shared/ui/glass";
import { useCopy } from "@/shared/hooks/useCopy";

/** A labelled, selectable URL with a copy button. */
export function UrlRow({ label, url }: { label: string; url: string }) {
  const { copied, copy } = useCopy();
  const { t } = useT();
  const done = copied === url;

  return (
    <div className="min-w-0 space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2">
        <code className="glass min-w-0 flex-1 truncate rounded-glass-sm px-3 py-2 font-mono text-sm select-text">
          {url}
        </code>
        <GlassButton size="sm" onClick={() => copy(url)} aria-label={t("connect.copyAria", { label })}>
          {done ? <Check /> : <Copy />}
          <span aria-live="polite">{done ? t("connect.copied") : t("connect.copy")}</span>
        </GlassButton>
      </div>
    </div>
  );
}
