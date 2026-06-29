/** Formatting helpers shared by every entity view — keep raw ISO strings and enum values off the screen. */

export function formatDate(value: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

export function formatRelative(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";

  const diffMinutes = (date.getTime() - Date.now()) / 60000;
  const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ["year", 525_600],
    ["month", 43_800],
    ["week", 10_080],
    ["day", 1_440],
    ["hour", 60],
    ["minute", 1],
  ];

  for (const [unit, minutesInUnit] of units) {
    if (Math.abs(diffMinutes) >= minutesInUnit) {
      return rtf.format(Math.round(diffMinutes / minutesInUnit), unit);
    }
  }
  return rtf.format(0, "minute");
}

export function formatSeconds(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = Math.round(totalSeconds % 60);
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

export function formatGrams(value: number | null): string {
  return value === null ? "—" : `${value.toFixed(1)} g`;
}

export function humanize(value: string): string {
  return value
    .split("_")
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}
