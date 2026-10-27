import { useState } from "react";
import { Plus, ShieldAlert, Users } from "lucide-react";
import { useLibraryStore } from "@/features/library";
import type { Profile } from "@/shared/bridge";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { MAX_PROFILES } from "../limits";
import { blankProfile, describeAccess, openProfileAdvice } from "../model";
import { useProfilesStore } from "../store";
import { Avatar } from "./Avatar";
import { ProfileEditor } from "./ProfileEditor";

/**
 * The people who share the library. Each can be limited to some libraries, kinds
 * or folders, and given a PIN. Go enforces every limit on every request; this
 * screen only edits them.
 */
export function ProfilesPanel() {
  const profiles = useProfilesStore((s) => s.profiles);
  const libraries = useLibraryStore((s) => s.libraries);
  // The ID being edited, "" for a new one, null for none.
  const [editing, setEditing] = useState<string | null>(null);
  if (!profiles) return null;

  const advice = openProfileAdvice(profiles);
  const shown: Profile | undefined =
    editing === "" ? blankProfile(profiles.map((p) => p.color)) : profiles.find((p) => p.id === editing);

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <Users className="size-5" aria-hidden /> Profiles
          </h2>
          <p className="text-sm text-muted-foreground">
            {profiles.length < 2
              ? "Add a profile for the kids, or for anyone who should see only part of the library. Devices then choose who is watching."
              : "A device chooses a profile when it connects. Each profile has its own “Continue watching”."}
          </p>
        </div>
        <GlassButton size="sm" onClick={() => setEditing("")} disabled={editing !== null || profiles.length >= MAX_PROFILES}>
          <Plus /> Add profile
        </GlassButton>
      </header>

      {advice && (
        <p role="status" className="flex items-start gap-2 text-sm text-warning">
          <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          {advice}
        </p>
      )}

      <ul className="flex flex-col gap-2">
        {profiles.map((p) =>
          editing === p.id && shown ? (
            <li key={p.id}>
              <ProfileEditor key={p.id} initial={shown} canDelete={profiles.length > 1} onClose={() => setEditing(null)} />
            </li>
          ) : (
            <li key={p.id} className="glass flex items-center gap-3 rounded-glass-md px-4 py-3">
              <Avatar name={p.name} color={p.color} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  {p.name} {p.hasPin && <span className="ml-1 text-xs text-muted-foreground">· PIN</span>}
                </p>
                <p className="truncate text-xs text-muted-foreground">{describeAccess(p, libraries)}</p>
              </div>
              <GlassButton size="sm" onClick={() => setEditing(p.id)} disabled={editing !== null}>
                Edit
              </GlassButton>
            </li>
          ),
        )}
        {editing === "" && shown && (
          <li>
            <ProfileEditor key="new" initial={shown} canDelete={false} onClose={() => setEditing(null)} />
          </li>
        )}
      </ul>
    </GlassPanel>
  );
}
