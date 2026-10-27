import type { LibraryState, MediaKind, Profile } from "@/shared/bridge";
import type { MessageKey, Translator } from "@/shared/i18n";
import { colorNames } from "./colors";

export const kindLabelKeys: Readonly<Record<MediaKind, MessageKey>> = {
  video: "profiles.kind.video",
  audio: "profiles.kind.audio",
  image: "profiles.kind.image",
};
export const allKinds: readonly MediaKind[] = ["video", "audio", "image"];

/** A new profile, not yet saved: open to everything, until the person narrows it. */
export function blankProfile(taken: readonly string[] = []): Profile {
  return {
    id: "",
    name: "",
    color: colorNames.find((c) => !taken.includes(c)) ?? colorNames[0] ?? "blue",
    allLibraries: true,
    libraries: [],
    kinds: [],
    hiddenFolders: [],
    hasPin: false,
  };
}

/** Hidden folder names as one per line, the way they are typed. */
export function foldersText(folders: readonly string[]): string {
  return folders.join("\n");
}

/** One name per line (or separated by commas); blanks and repeats dropped, order kept. */
export function parseFolders(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const part of text.split(/[\n,]/)) {
    const name = part.trim();
    const key = name.toLowerCase();
    if (name !== "" && !seen.has(key)) {
      seen.add(key);
      out.push(name);
    }
  }
  return out;
}

/** Ticks or unticks one library. Ticking all of them is the same as "all libraries", and Go would see a list that misses future libraries, so the caller decides. */
export function toggleLibrary(profile: Profile, id: string): Profile {
  const libraries = profile.libraries.includes(id)
    ? profile.libraries.filter((l) => l !== id)
    : [...profile.libraries, id];
  return { ...profile, libraries };
}

/**
 * Ticks or unticks one kind. An empty list means "every kind", so unticking the
 * last one would silently allow everything: that click does nothing instead. From
 * "every kind" (an empty list), unticking one starts a list of the others.
 */
export function toggleKind(profile: Profile, kind: MediaKind): Profile {
  let kinds: MediaKind[];
  if (profile.kinds.length === 0) {
    kinds = allKinds.filter((k) => k !== kind);
  } else if (profile.kinds.includes(kind)) {
    kinds = profile.kinds.filter((k) => k !== kind);
    if (kinds.length === 0) return profile;
  } else {
    kinds = [...profile.kinds, kind];
  }
  // Keep Go's order, so the same choice always saves the same way.
  return { ...profile, kinds: allKinds.filter((k) => kinds.includes(k)) };
}

/** A profile as one line: "All libraries · Videos only · hides 2 folders". */
export function describeAccess(p: Profile, libraries: readonly LibraryState[], tr: Translator): string {
  const parts: string[] = [];
  if (p.allLibraries) {
    parts.push(tr.t("profiles.access.all"));
  } else {
    const names = p.libraries.map((id) => libraries.find((l) => l.id === id)?.name).filter((n): n is string => !!n);
    parts.push(names.length === 0 ? tr.t("profiles.access.none") : names.join(", "));
  }
  if (p.kinds.length > 0 && p.kinds.length < allKinds.length) {
    parts.push(tr.t("profiles.access.only", { kinds: tr.list(p.kinds.map((k) => tr.t(kindLabelKeys[k]))) }));
  }
  if (p.hiddenFolders.length > 0) {
    parts.push(tr.tn("profiles.access.hides", p.hiddenFolders.length));
  }
  return parts.join(" · ");
}

/**
 * Whether the profiles need a word of advice: with several profiles, anyone can
 * pick one that has no PIN, so a profile that sees everything and has none is the
 * way round the others.
 */
export function openProfileAdvice(profiles: readonly Profile[], tr: Translator): string | null {
  if (profiles.length < 2) return null;
  const open = profiles.filter((p) => p.allLibraries && p.kinds.length === 0 && p.hiddenFolders.length === 0 && !p.hasPin);
  if (open.length === 0) return null;
  const names = tr.list(open.map((p) => `“${p.name}”`));
  return tr.tn("profiles.advice", open.length, { names });
}

/** Libraries a profile that lists libraries no longer has, so the editor does not show ticks for ghosts. */
export function liveLibraries(p: Profile, libraries: readonly LibraryState[]): string[] {
  return p.libraries.filter((id) => libraries.some((l) => l.id === id));
}

/** A PIN is four to six digits; checked here only to disable the button, Go decides. */
export function pinLooksValid(pin: string): boolean {
  return /^\d{4,6}$/.test(pin);
}
