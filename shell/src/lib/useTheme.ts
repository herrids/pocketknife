import { useSyncExternalStore, useCallback } from "react";

export type Theme = "light" | "dark";

const STORAGE_KEY = "pk_theme";

function systemTheme(): Theme {
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function readStoredTheme(): Theme {
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored === "light" || stored === "dark" ? stored : systemTheme();
}

function applyTheme(t: Theme) {
  document.documentElement.classList.toggle("dark", t === "dark");
}

let theme = readStoredTheme();
applyTheme(theme);

const listeners = new Set<() => void>();

function setTheme(t: Theme) {
  theme = t;
  localStorage.setItem(STORAGE_KEY, t);
  applyTheme(t);
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

// Shared across every component that calls it, via an external store rather
// than context, since the toggle button and the screens applying `dark:`
// classes don't otherwise share a common ancestor.
export function useTheme(): [Theme, () => void] {
  const current = useSyncExternalStore(subscribe, () => theme);
  const toggle = useCallback(() => setTheme(current === "dark" ? "light" : "dark"), [current]);
  return [current, toggle];
}
