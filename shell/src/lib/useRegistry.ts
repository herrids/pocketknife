import { useState, useEffect, useCallback } from "react";
import { api, type RegistryEntry } from "./api";

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

  // Poll every 3 s so a build that starts after mount (e.g. the agent is
  // still planning when the shell navigates back to Home) is picked up
  // without requiring an app to already be in an active state at mount time.
  useEffect(() => {
    const id = setInterval(load, 3000);
    return () => clearInterval(id);
  }, [load]);

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
