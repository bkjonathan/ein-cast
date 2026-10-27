import { useState } from "react";
import { useServerStore } from "@/features/server";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Input } from "@/shared/ui/input";
import type { SettingsInput } from "@/shared/bridge";
import { formFrom, inputFrom, isDirty, needsRestart, parsePort } from "../dirty";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { useSettingsStore } from "../store";

/** Port and display name. Saved values come back from Go; the form edits a copy. */
export function ServerSettings() {
  const saved = useSettingsStore((s) => s.settings);
  if (!saved) return null;
  // Re-key on the saved values so the form resets to exactly what Go stored.
  return <ServerForm key={`${saved.port}|${saved.name}`} saved={saved} />;
}

function ServerForm({ saved }: { saved: SettingsInput }) {
  const [form, setForm] = useState(() => formFrom(saved));
  const { save, pending, error } = useSettingsCommands();
  const { t } = useT();
  const running = useServerStore((s) => s.status);
  const dirty = isDirty(form, saved);

  return (
    <GlassPanel className="p-6">
      <form
        className="flex flex-col gap-4"
        // A real form, so Enter saves.
        onSubmit={(e) => {
          e.preventDefault();
          void save(inputFrom(saved, { port: parsePort(form.port), name: form.name }));
        }}
      >
        <header>
          <h2 className="text-lg font-semibold">{t("settings.server.title")}</h2>
          <p className="text-sm text-muted-foreground">{t("settings.server.intro")}</p>
        </header>

        <div className="grid gap-4 sm:grid-cols-[1fr_9rem]">
          <label className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t("settings.server.displayName")}</span>
            <Input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              maxLength={64}
              autoComplete="off"
            />
          </label>
          <label className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t("settings.server.port")}</span>
            <Input
              value={form.port}
              onChange={(e) => setForm({ ...form, port: e.target.value })}
              inputMode="numeric"
              autoComplete="off"
              className="font-mono"
            />
          </label>
        </div>

        {needsRestart(running, saved) && (
          <p className="text-sm text-warning">
            {t("settings.server.restartNote")}
          </p>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}

        <div className="flex gap-2">
          <GlassButton type="submit" disabled={!dirty || pending}>
            {t("common.save")}
          </GlassButton>
          {dirty && (
            <GlassButton type="button" onClick={() => setForm(formFrom(saved))} disabled={pending}>
              {t("common.discard")}
            </GlassButton>
          )}
        </div>
      </form>
    </GlassPanel>
  );
}
