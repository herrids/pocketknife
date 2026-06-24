// Small presentational helpers. Kept apart from the API client so the
// generated client.ts stays a pure, regenerable artifact.
import type { ExerciseKind, ExerciseStation, WorkoutType } from "./client";

export const WORKOUT_TYPES: WorkoutType[] = ["hyrox", "strength", "running", "mixed", "other"];

export const EXERCISE_KINDS: ExerciseKind[] = ["run", "hyrox_station", "strength", "cardio", "mobility", "other"];

export const HYROX_STATIONS: ExerciseStation[] = [
  "ski_erg",
  "sled_push",
  "sled_pull",
  "burpee_broad_jump",
  "row",
  "farmers_carry",
  "sandbag_lunges",
  "wall_balls",
];

const TYPE_EMOJI: Record<WorkoutType, string> = {
  hyrox: "🟦",
  strength: "🏋️",
  running: "🏃",
  mixed: "🔀",
  other: "•",
};

export function typeEmoji(t: WorkoutType | null | undefined): string {
  return (t && TYPE_EMOJI[t]) ?? "•";
}

// Replace the underscores in enum values with spaces for display. Tolerates
// null because nullable enum fields surface as `T | null` in the typed client.
export function humanize(value: string | null | undefined): string {
  return (value ?? "").replace(/_/g, " ");
}

// Seconds -> "h:mm:ss" or "m:ss". Returns "—" for null/0-less input.
export function formatDuration(seconds: number | null | undefined): string {
  if (seconds == null || seconds <= 0) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  const pad = (n: number) => String(n).padStart(2, "0");
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
}

// "1:23:45" / "23:45" / "45" / "90s" -> total seconds, or null if blank/invalid.
export function parseDuration(input: string): number | null {
  const raw = input.trim();
  if (raw === "") return null;
  if (/^\d+s$/i.test(raw)) return parseInt(raw, 10);
  if (/^\d+$/.test(raw)) return parseInt(raw, 10);
  const parts = raw.split(":").map((p) => p.trim());
  if (parts.length < 2 || parts.length > 3 || parts.some((p) => !/^\d+$/.test(p))) return null;
  return parts.reduce((acc, p) => acc * 60 + parseInt(p, 10), 0);
}

// A <input type="datetime-local"> value (local time) -> ISO string for the API.
export function localToISO(local: string): string {
  if (!local) return "";
  const d = new Date(local);
  return Number.isNaN(d.getTime()) ? "" : d.toISOString();
}

// An ISO string from the API -> a <input type="datetime-local"> value.
export function isoToLocalInput(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString(undefined, { weekday: "short", year: "numeric", month: "short", day: "numeric" });
}

export function todayLocalInput(): string {
  return isoToLocalInput(new Date().toISOString());
}
