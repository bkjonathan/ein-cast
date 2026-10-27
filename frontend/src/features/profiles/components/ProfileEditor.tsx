import { useState } from "react";
import { KeyRound, Trash2 } from "lucide-react";
import { useLibraryStore } from "@/features/library";
import type { Profile } from "@/shared/bridge";
import { cn } from "@/shared/lib/cn";
import { GlassButton } from "@/shared/ui/glass";
import { Input } from "@/shared/ui/input";
import { colorNames, colorOf } from "../colors";
import { useProfileCommands } from "../hooks/useProfileCommands";
import { allKinds, foldersText, kindLabels, parseFolders, pinLooksValid, toggleKind, toggleLibrary } from "../model";

interface ProfileEditorProps {
  /** The profile to edit, or a blank one to add. */
  initial: Profile;
  /** Whether it can be deleted: not the last one, and not one that has not been saved. */
  canDelete: boolean;
  onClose: () => void;
}

/** The form for one profile: who, what it may see, and its PIN. */
export function ProfileEditor({ initial, canDelete, onClose }: ProfileEditorProps) {
  const libraries = useLibraryStore((s) => s.libraries);
  const [draft, setDraft] = useState<Profile>(initial);
  const [folders, setFolders] = useState(() => foldersText(initial.hiddenFolders));
  const [pin, setPin] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const commands = useProfileCommands();
  const isNew = initial.id === "";

  const current = (): Profile => ({ ...draft, hiddenFolders: parseFolders(folders) });

  async function save() {
    const saved = await commands.save(current());
    if (!saved) return;
    // A PIN typed while adding is set on the profile Go just created.
    if (pin !== "" && !(await commands.setPin(saved.id, pin))) return;
    onClose();
  }

  return (
    <form
      className="glass flex flex-col gap-4 rounded-glass-md p-4"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <div className="grid gap-4 sm:grid-cols-[1fr_auto]">
        <label className="space-y-1.5">
          <span className="text-xs font-medium text-muted-foreground">Name</span>
          <Input
            autoFocus
            value={draft.name}
            maxLength={32}
            autoComplete="off"
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="Kids"
          />
        </label>
        <fieldset className="space-y-1.5">
          <legend className="text-xs font-medium text-muted-foreground">Colour</legend>
          <div className="flex flex-wrap gap-2">
            {colorNames.map((c) => (
              <button
                key={c}
                type="button"
                aria-label={c}
                aria-pressed={draft.color === c}
                onClick={() => setDraft({ ...draft, color: c })}
                className={cn("app-no-drag size-7 rounded-full ring-offset-2 ring-offset-transparent", draft.color === c && "ring-2 ring-foreground")}
                style={{ background: colorOf(c) }}
              />
            ))}
          </div>
        </fieldset>
      </div>

      <fieldset className="space-y-2">
        <legend className="text-xs font-medium text-muted-foreground">Libraries it can see</legend>
        <Check
          checked={draft.allLibraries}
          onChange={(on) => setDraft({ ...draft, allLibraries: on })}
          label="All libraries, including ones added later"
        />
        {!draft.allLibraries &&
          libraries.map((l) => (
            <Check
              key={l.id}
              checked={draft.libraries.includes(l.id)}
              onChange={() => setDraft(toggleLibrary(draft, l.id))}
              label={l.name}
              indent
            />
          ))}
        {!draft.allLibraries && draft.libraries.length === 0 && (
          <p className="text-xs text-warning">With no library ticked, this profile sees nothing.</p>
        )}
      </fieldset>

      <fieldset className="space-y-2">
        <legend className="text-xs font-medium text-muted-foreground">What it can see</legend>
        <div className="flex flex-wrap gap-4">
          {allKinds.map((k) => (
            <Check
              key={k}
              checked={draft.kinds.length === 0 || draft.kinds.includes(k)}
              onChange={() => setDraft(toggleKind(draft, k))}
              label={kindLabels[k]}
            />
          ))}
        </div>
      </fieldset>

      <label className="space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">Hide folders named (one per line)</span>
        <textarea
          value={folders}
          onChange={(e) => setFolders(e.target.value)}
          rows={3}
          placeholder={"Adults\n18*"}
          className="glass w-full resize-y rounded-glass-sm px-3 py-2 font-mono text-sm outline-none"
        />
        <span className="block text-xs text-muted-foreground">
          Hides everything inside a folder with that name, at any depth. <code>*</code> and <code>?</code> are wildcards.
        </span>
      </label>

      <div className="space-y-2">
        <p className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
          <KeyRound className="size-3.5" aria-hidden /> PIN
          {initial.hasPin && <span className="text-success">set</span>}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            value={pin}
            onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 6))}
            inputMode="numeric"
            autoComplete="off"
            placeholder={initial.hasPin ? "New PIN" : "4 to 6 digits"}
            aria-label="PIN"
            className="w-40 font-mono"
          />
          {!isNew && pin !== "" && (
            <GlassButton
              size="sm"
              type="button"
              disabled={commands.pending || !pinLooksValid(pin)}
              onClick={async () => {
                if (await commands.setPin(initial.id, pin)) setPin("");
              }}
            >
              {initial.hasPin ? "Change PIN" : "Set PIN"}
            </GlassButton>
          )}
          {initial.hasPin && (
            <GlassButton size="sm" type="button" disabled={commands.pending} onClick={() => void commands.clearPin(initial.id)}>
              Remove PIN
            </GlassButton>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          Asked when someone picks this profile on a TV, phone or tablet. It stops a curious child, not someone who can read the files on this computer.
        </p>
      </div>

      {commands.error && (
        <p role="alert" className="text-sm text-destructive">
          {commands.error}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <GlassButton type="submit" disabled={commands.pending || draft.name.trim() === ""}>
          {isNew ? "Add profile" : "Save"}
        </GlassButton>
        <GlassButton type="button" onClick={onClose} disabled={commands.pending}>
          Cancel
        </GlassButton>
        {canDelete &&
          (confirmDelete ? (
            <span className="ml-auto flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Delete “{initial.name}”? Its “Continue watching” goes too.</span>
              <GlassButton
                size="sm"
                type="button"
                disabled={commands.pending}
                onClick={async () => {
                  if (await commands.remove(initial.id)) onClose();
                }}
              >
                <Trash2 /> Delete
              </GlassButton>
              <GlassButton size="sm" type="button" onClick={() => setConfirmDelete(false)}>
                Keep
              </GlassButton>
            </span>
          ) : (
            <GlassButton size="sm" type="button" className="ml-auto" onClick={() => setConfirmDelete(true)}>
              <Trash2 /> Delete…
            </GlassButton>
          ))}
      </div>
    </form>
  );
}

function Check({ checked, onChange, label, indent }: { checked: boolean; onChange: (on: boolean) => void; label: string; indent?: boolean }) {
  return (
    <label className={cn("flex cursor-pointer items-center gap-2 text-sm", indent && "pl-6")}>
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="size-4 accent-(--color-primary)" />
      {label}
    </label>
  );
}
