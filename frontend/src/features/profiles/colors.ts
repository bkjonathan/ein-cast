/** The avatar colours, by the names Go stores. Go accepts only these names, so nothing else ever reaches a style. */
export const profileColors: Readonly<Record<string, string>> = {
  blue: "#3b82f6",
  green: "#22c55e",
  red: "#ef4444",
  orange: "#f97316",
  purple: "#a855f7",
  pink: "#ec4899",
  teal: "#14b8a6",
  gray: "#6b7280",
};

export const colorNames: readonly string[] = Object.keys(profileColors);

/** The colour for a name, or gray for one this version does not know. */
export function colorOf(name: string): string {
  return profileColors[name] ?? profileColors["gray"] ?? "#6b7280";
}
