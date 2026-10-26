// Mirrors the rules in internal/app/remote/pin.go so the form can say what is
// wrong as the person types. Go still decides: it is the one that enforces them.
export const minPinLength = 8;
export const maxPinLength = 64;

/** What is wrong with a PIN, or null when it is acceptable. */
export function pinProblem(pin: string): string | null {
  // Count characters, not UTF-16 units, as Go counts runes.
  const length = [...pin].length;
  if (length < minPinLength) return `Use at least ${minPinLength} characters.`;
  if (length > maxPinLength) return `Use at most ${maxPinLength} characters.`;
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f-\u009f]/.test(pin)) return "Control characters are not allowed.";
  return null;
}
