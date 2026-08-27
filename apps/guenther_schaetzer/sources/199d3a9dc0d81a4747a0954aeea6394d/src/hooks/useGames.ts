import { useState, useEffect, useCallback } from "react";
import { toast } from "sonner";
import { ApiError } from "@/client";
import type { Game, GameCreateInput, GameUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useGames() {
  const [games, setGames] = useState<Game[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await client.game.list({ sort: ["-created_at"], limit: 100 });
      setGames(result.data);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Laden der Spiele");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = useCallback(async (input: GameCreateInput): Promise<Game> => {
    const g = await client.game.create(input);
    setGames((prev) => [g, ...prev]);
    toast.success("Spiel gestartet!");
    return g;
  }, []);

  const update = useCallback(async (id: string, input: GameUpdateInput): Promise<Game> => {
    const g = await client.game.update(id, input);
    setGames((prev) => prev.map((x) => (x.id === id ? g : x)));
    return g;
  }, []);

  const remove = useCallback(async (id: string): Promise<void> => {
    await client.game.delete(id);
    setGames((prev) => prev.filter((x) => x.id !== id));
    toast.success("Spiel gelöscht");
  }, []);

  return { games, loading, refetch: load, create, update, remove };
}
