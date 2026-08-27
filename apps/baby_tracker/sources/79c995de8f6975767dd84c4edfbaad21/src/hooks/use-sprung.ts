import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiError, Sprung, SprungCreateInput, SprungUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useSprung() {
  const [items, setItems] = useState<Sprung[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await client.sprung.list({ sort: ["nummer"] });
      setItems(res.data);
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const create = useCallback(async (input: SprungCreateInput): Promise<boolean> => {
    try {
      const item = await client.sprung.create(input);
      setItems((prev) => [...prev, item].sort((a, b) => a.nummer - b.nummer));
      toast.success(`Sprung ${item.nummer} hinzugefügt`);
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const update = useCallback(async (id: string, input: SprungUpdateInput): Promise<boolean> => {
    try {
      const updated = await client.sprung.update(id, input);
      setItems((prev) =>
        prev.map((s) => (s.id === id ? updated : s)).sort((a, b) => a.nummer - b.nummer),
      );
      toast.success("Sprung aktualisiert");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const remove = useCallback(async (id: string): Promise<boolean> => {
    try {
      await client.sprung.delete(id);
      setItems((prev) => prev.filter((s) => s.id !== id));
      toast.success("Sprung gelöscht");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const byId = useMemo(() => new Map(items.map((s) => [s.id, s])), [items]);

  return { items, loading, create, update, remove, byId };
}
