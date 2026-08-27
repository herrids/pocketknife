import type { QuestionCategory, QuestionDifficulty } from "@/client";

export const CATEGORY_LABELS: Record<QuestionCategory, string> = {
  stadion: "Stadion",
  tore: "Tore",
  karten: "Karten",
  vereine: "Vereine",
  transfers: "Transfers",
  punkte: "Punkte",
  geschichte: "Geschichte",
  international: "International",
};

export const CATEGORY_EMOJIS: Record<QuestionCategory, string> = {
  stadion: "🏟",
  tore: "⚽",
  karten: "🟨",
  vereine: "🛡",
  transfers: "💸",
  punkte: "📊",
  geschichte: "📜",
  international: "🌍",
};

export const CATEGORY_COLORS: Record<QuestionCategory, string> = {
  stadion: "bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300",
  tore: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300",
  karten: "bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-300",
  vereine: "bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300",
  transfers: "bg-orange-100 text-orange-800 dark:bg-orange-950 dark:text-orange-300",
  punkte: "bg-teal-100 text-teal-800 dark:bg-teal-950 dark:text-teal-300",
  geschichte: "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300",
  international: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300",
};

export const DIFFICULTY_LABELS: Record<QuestionDifficulty, string> = {
  leicht: "Leicht",
  mittel: "Mittel",
  schwer: "Schwer",
};

export const DIFFICULTY_COLORS: Record<QuestionDifficulty, string> = {
  leicht: "bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300",
  mittel: "bg-amber-100 text-amber-700 dark:bg-amber-950 dark:text-amber-300",
  schwer: "bg-rose-100 text-rose-700 dark:bg-rose-950 dark:text-rose-300",
};
