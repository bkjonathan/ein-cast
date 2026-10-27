import { useState } from "react";
import { KeyRound, Trash2 } from "lucide-react";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Input } from "@/shared/ui/input";
import { useRemoteCommands } from "../hooks/useRemoteCommands";
import { maxPinLength, pinProblem } from "../pin";
import { useRemoteStore } from "../store";

/**
 * The access PIN that public providers require. Shown only when it can matter:
 * a public provider is offered, or a PIN already exists.
 */
export function PinPanel() {
  const status = useRemoteStore((s) => s.status);
  const offersPublic = useRemoteStore((s) => s.providers.some((p) => p.public));
  const { setPin, clearPin, busy, error } = useRemoteCommands();
  const [editing, setEditing] = useState(false);
  const [pin, setPinText] = useState("");
  const tr = useT();
  const { t } = tr;

  if (!status || !(offersPublic || status.pinSet)) return null;

  const showForm = !status.pinSet || editing;
  const problem = pin === "" ? null : pinProblem(pin, tr);

  async function submit() {
    if (pinProblem(pin, tr) !== null) return;
    if (await setPin(pin)) {
      setPinText("");
      setEditing(false);
    }
  }

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold">{t("remote.pin.title")}</h2>
        <p className="text-sm text-muted-foreground">
          {t("remote.pin.intro")}
        </p>
      </header>

      {showForm ? (
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <label className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">
              {status.pinSet ? t("remote.pin.new") : t("remote.pin.label")}
            </span>
            <Input
              type="password"
              value={pin}
              onChange={(e) => setPinText(e.target.value)}
              maxLength={maxPinLength + 8}
              autoComplete="new-password"
              aria-invalid={problem !== null}
              aria-describedby="pin-help"
            />
          </label>
          <p id="pin-help" className={problem ? "text-sm text-destructive" : "text-sm text-muted-foreground"}>
            {problem ?? t("remote.pin.hint")}
          </p>
          {status.pinSet && (
            <p className="text-sm text-muted-foreground">{t("remote.pin.signsOut")}</p>
          )}
          <div className="flex gap-2">
            <GlassButton type="submit" disabled={busy.pin || pin === "" || problem !== null}>
              <KeyRound /> {status.pinSet ? t("remote.pin.change") : t("remote.pin.set")}
            </GlassButton>
            {editing && (
              <GlassButton
                type="button"
                onClick={() => {
                  setEditing(false);
                  setPinText("");
                }}
              >
                {t("common.cancel")}
              </GlassButton>
            )}
          </div>
        </form>
      ) : (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-sm">{t("remote.pin.isSet")}</p>
          <GlassButton size="sm" onClick={() => setEditing(true)}>
            <KeyRound /> {t("remote.pin.changeAsk")}
          </GlassButton>
          <GlassButton
            size="sm"
            onClick={() => void clearPin()}
            // Go refuses too; this just explains why instead of failing.
            disabled={busy.pin || status.public}
            title={status.public ? t("remote.pin.removeBlocked") : undefined}
          >
            <Trash2 /> {t("remote.pin.remove")}
          </GlassButton>
          {status.public && (
            <p className="text-sm text-muted-foreground">{t("remote.pin.removeBlockedShort")}</p>
          )}
        </div>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive select-text">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
