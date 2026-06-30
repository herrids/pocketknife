import { useState, useEffect, useCallback } from "react";
import { api, type RegistryEntry } from "./api";

const ACTIVE_STATES = new Set(["queued", "building", "activating"]);

export function useRegistry() {
  const [entries, setEntries] = useState<RegistryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await api.registry();
      setEntries(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load apps");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  // Poll every 3 s while any app is in an active build state.
  useEffect(() => {
    const hasActive = entries.some((e) => ACTIVE_STATES.has(e.buildState));
    if (!hasActive) return;

    const id = setInterval(load, 3000);
    return () => clearInterval(id);
  }, [entries, load]);

  return { entries, loading, error, reload: load };
}

export function useLastOpenedApp(): [string | null, (appId: string) => void] {
  const [lastApp, setLastApp] = useState<string | null>(() =>
    localStorage.getItem("pk_last_opened"),
  );

  const recordOpen = useCallback((appId: string) => {
    localStorage.setItem("pk_last_opened", appId);
    setLastApp(appId);
  }, []);

  return [lastApp, recordOpen];
}
