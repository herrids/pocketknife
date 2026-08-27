import { FaehigkeitKategorie } from "@/client";

export const KATEGORIE_LABELS: Record<FaehigkeitKategorie, string> = {
  motorik: "Motorik",
  sprache: "Sprache",
  soziales: "Soziales",
  wahrnehmung: "Wahrnehmung",
  emotion: "Emotion",
};

export const KATEGORIE_COLORS: Record<FaehigkeitKategorie, string> = {
  motorik:
    "bg-blue-100 text-blue-700 border-blue-200 dark:bg-blue-900/30 dark:text-blue-300 dark:border-blue-800",
  sprache:
    "bg-emerald-100 text-emerald-700 border-emerald-200 dark:bg-emerald-900/30 dark:text-emerald-300 dark:border-emerald-800",
  soziales:
    "bg-orange-100 text-orange-700 border-orange-200 dark:bg-orange-900/30 dark:text-orange-300 dark:border-orange-800",
  wahrnehmung:
    "bg-violet-100 text-violet-700 border-violet-200 dark:bg-violet-900/30 dark:text-violet-300 dark:border-violet-800",
  emotion:
    "bg-rose-100 text-rose-700 border-rose-200 dark:bg-rose-900/30 dark:text-rose-300 dark:border-rose-800",
};

export const KATEGORIE_OPTIONS: FaehigkeitKategorie[] = [
  "motorik",
  "sprache",
  "soziales",
  "wahrnehmung",
  "emotion",
];
