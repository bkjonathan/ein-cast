const units = ["B", "KB", "MB", "GB", "TB"] as const;

/**
 * Formats a byte count for people: 1536 → "1.5 KB". Uses 1024-based steps. The
 * number follows the language's separators (a decimal comma in German), but always
 * in Latin digits: Burmese digits are a choice, not a default, and the units stay
 * KB/MB/GB as people read them everywhere.
 */
export function formatBytes(bytes: number, locale = "en"): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = unit === 0 || value >= 10 ? 0 : 1;
  const number = new Intl.NumberFormat(`${locale}-u-nu-latn`, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
    useGrouping: false,
  }).format(value);
  return `${number} ${units[unit]}`;
}

/**
 * Formats a moment (Unix milliseconds) as a short date, e.g. "Nov 1, 2026". Latin
 * digits in every language, like formatBytes. A moment of 0 means "unknown" and
 * gives a dash.
 */
export function formatDate(ms: number, locale = "en"): string {
  if (!Number.isFinite(ms) || ms <= 0) return "—";
  return new Intl.DateTimeFormat(`${locale}-u-nu-latn`, { dateStyle: "medium" }).format(ms);
}

/** Like formatDate, with the time of day: "Nov 1, 2026, 9:05 AM". */
export function formatDateTime(ms: number, locale = "en"): string {
  if (!Number.isFinite(ms) || ms <= 0) return "—";
  return new Intl.DateTimeFormat(`${locale}-u-nu-latn`, { dateStyle: "medium", timeStyle: "short" }).format(ms);
}
