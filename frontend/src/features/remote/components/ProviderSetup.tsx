import { useState } from "react";
import { Check, FolderOpen, KeyRound, Trash2 } from "lucide-react";
import type { RemoteField, RemoteFieldState, RemoteProvider } from "@/shared/bridge";
import { useT } from "@/shared/i18n";
import { GlassButton } from "@/shared/ui/glass";
import { Input } from "@/shared/ui/input";
import { useProviderConfig } from "../hooks/useProviderConfig";
import { describeProvider, setupNote } from "../providers";

interface ProviderSetupProps {
  provider: RemoteProvider;
  /** The provider is on, so its settings cannot change. */
  inUse: boolean;
  /** The server's configured port, for advice that mentions it. */
  port: number | undefined;
  /** Changes whenever Go may have changed the settings behind our back. */
  refreshOn: unknown;
}

/**
 * The settings a provider needs: a token, a host name, where a program is. A
 * provider whose settings are all optional gets them under "Advanced", closed.
 */
export function ProviderSetup({ provider, inUse, port, refreshOn }: ProviderSetupProps) {
  const config = useProviderConfig(provider.id, refreshOn);
  const tr = useT();
  const { t } = tr;
  const label = describeProvider(provider).label;
  const note = setupNote(provider.id, port, tr);
  const needsSomething = provider.fields.some((f) => f.required);

  const body = (
    <div className="flex flex-col gap-4">
      {inUse && (
        <p className="text-sm text-muted-foreground">{t("remote.setup.turnOffFirst", { label })}</p>
      )}
      {note && <p className="text-sm text-muted-foreground">{note}</p>}

      {provider.fields.map((field) => {
        const state = config.states?.find((s) => s.key === field.key);
        return (
          // Re-keyed on what is saved, so a row resets to exactly what Go stored.
          <FieldRow
            key={`${provider.id}:${field.key}:${state?.set}:${state?.value}`}
            field={field}
            state={state}
            disabled={inUse || config.busyKey !== null}
            busy={config.busyKey === field.key}
            onSave={(value) => config.save(field.key, value)}
            onRemove={() => config.remove(field.key)}
            onBrowse={async () => {
              const path = await config.browse(t("remote.setup.choose", { field: field.label }));
              if (path) await config.save(field.key, path);
            }}
          />
        );
      })}

      {config.error && (
        <p role="alert" className="text-sm text-destructive select-text">
          {config.error}
        </p>
      )}
    </div>
  );

  if (needsSomething) {
    return (
      <section aria-label={t("remote.setup.title", { label })} className="glass flex flex-col gap-4 rounded-glass-sm p-4">
        <h3 className="font-medium">{t("remote.setup.title", { label })}</h3>
        {body}
      </section>
    );
  }
  return (
    <details className="glass rounded-glass-sm p-4">
      <summary className="app-no-drag cursor-pointer text-sm font-medium">{t("remote.setup.advanced")}</summary>
      <div className="mt-4">{body}</div>
    </details>
  );
}

interface FieldRowProps {
  field: RemoteField;
  state: RemoteFieldState | undefined;
  disabled: boolean;
  busy: boolean;
  onSave: (value: string) => Promise<boolean>;
  onRemove: () => Promise<boolean>;
  onBrowse: () => Promise<void>;
}

function FieldRow({ field, state, disabled, busy, onSave, onRemove, onBrowse }: FieldRowProps) {
  const { t } = useT();
  const isSecret = field.kind === "secret";
  const isSet = state?.set === true;
  const [draft, setDraft] = useState(isSecret ? "" : (state?.value ?? ""));
  const [replacing, setReplacing] = useState(false);
  const id = `setup-${field.key}`;

  // A saved secret is never shown: only that it is there.
  const showSavedSecret = isSecret && isSet && !replacing;

  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        {field.label}
        {field.required && <span className="rounded-full bg-primary/20 px-2 py-0.5 text-primary">{t("remote.setup.required")}</span>}
        {isSet && <Check className="size-3.5 text-success" aria-label={t("remote.setup.saved")} />}
      </label>

      {showSavedSecret ? (
        <div className="flex flex-wrap items-center gap-2">
          <span className="glass flex items-center gap-2 rounded-glass-sm px-3 py-2 text-sm">
            <KeyRound className="size-4" aria-hidden /> {t("remote.setup.inKeychain")}
          </span>
          <GlassButton size="sm" disabled={disabled} onClick={() => setReplacing(true)}>
            {t("remote.setup.replace")}
          </GlassButton>
          <GlassButton size="sm" disabled={disabled} onClick={() => void onRemove()}>
            <Trash2 /> {t("remote.setup.remove")}
          </GlassButton>
        </div>
      ) : (
        <form
          className="flex flex-wrap items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (draft.trim() !== "") void onSave(draft);
          }}
        >
          <Input
            id={id}
            type={isSecret ? "password" : "text"}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={field.placeholder}
            disabled={disabled}
            autoComplete="off"
            spellCheck={false}
            className="min-w-0 flex-1 basis-56"
          />
          {field.kind === "file" && (
            <GlassButton type="button" size="sm" disabled={disabled} onClick={() => void onBrowse()}>
              <FolderOpen /> {t("remote.setup.browse")}
            </GlassButton>
          )}
          <GlassButton type="submit" size="sm" disabled={disabled || busy || draft.trim() === ""}>
            {t("common.save")}
          </GlassButton>
          {replacing && (
            <GlassButton type="button" size="sm" onClick={() => setReplacing(false)}>
              {t("common.cancel")}
            </GlassButton>
          )}
          {isSet && !isSecret && (
            <GlassButton type="button" size="sm" disabled={disabled} onClick={() => void onRemove()}>
              <Trash2 /> {t("remote.setup.remove")}
            </GlassButton>
          )}
        </form>
      )}
      {field.help && <p className="text-xs text-muted-foreground">{field.help}</p>}
    </div>
  );
}
