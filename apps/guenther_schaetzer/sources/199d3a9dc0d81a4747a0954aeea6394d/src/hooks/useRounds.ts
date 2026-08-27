import { useState, useEffect, useCallback } from "react";
import { toast } from "sonner";
import { ApiError } from "@/client";
import type { Round, RoundCreateInput, RoundUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useRounds(gameId: string) {
  const [rounds, setRounds] = useState<Round[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    if (!gameId) return;
    setLoading(true);
    try {
      const result = await client.round.list({
        filter: [["game", "eq", gameId]],
        sort: ["round_number"],
        limit: 100,
      });
      setRounds(result.data);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Laden der Runden");
    } finally {
      setLoading(false);
    }
  }, [gameId]);

  useEffect(() => {
    void load();
  }, [load]);

  const create = useCallback(async (input: RoundCreateInput): Promise<Round> => {
    const r = await client.round.create(input);
    setRounds((prev) => [...prev, r].sort((a, b) => a.round_number - b.round_number));
    return r;
  }, []);

  const update = useCallback(async (id: string, input: RoundUpdateInput): Promise<Round> => {
    const r = await client.round.update(id, input);
    setRounds((prev) => prev.map((x) => (x.id === id ? r : x)));
    return r;
  }, []);

  const remove = useCallback(async (id: string): Promise<void> => {
    await client.round.delete(id);
    setRounds((prev) => prev.filter((x) => x.id !== id));
  }, []);

  return { rounds, loading, refetch: load, create, update, remove };
}
