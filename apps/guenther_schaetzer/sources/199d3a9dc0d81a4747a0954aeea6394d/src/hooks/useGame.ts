import { useState, useEffect, useCallback } from "react";
import { toast } from "sonner";
import { ApiError } from "@/client";
import type { Game, GameUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useGame(id: string) {
  const [game, setGame] = useState<Game | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const g = await client.game.get(id);
      setGame(g);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Laden des Spiels");
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    void load();
  }, [load]);

  const update = useCallback(
    async (input: GameUpdateInput): Promise<Game | null> => {
      if (!game) return null;
      try {
        const g = await client.game.update(game.id, input);
        setGame(g);
        return g;
      } catch (err) {
        toast.error(err instanceof ApiError ? err.message : "Fehler beim Speichern");
        return null;
      }
    },
    [game],
  );

  return { game, loading, refetch: load, update };
}
