import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiError, Baby, BabyCreateInput, BabyUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useBabies() {
  const [items, setItems] = useState<Baby[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await client.baby.list({ sort: ["name"] });
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

  const create = useCallback(async (input: BabyCreateInput): Promise<boolean> => {
    try {
      const item = await client.baby.create(input);
      setItems((prev) => [...prev, item].sort((a, b) => a.name.localeCompare(b.name)));
      toast.success(`${item.name} hinzugefügt`);
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const update = useCallback(async (id: string, input: BabyUpdateInput): Promise<boolean> => {
    try {
      const updated = await client.baby.update(id, input);
      setItems((prev) => prev.map((b) => (b.id === id ? updated : b)));
      toast.success("Profil aktualisiert");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const remove = useCallback(async (id: string): Promise<boolean> => {
    try {
      await client.baby.delete(id);
      setItems((prev) => prev.filter((b) => b.id !== id));
      toast.success("Profil gelöscht");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const byId = useMemo(() => new Map(items.map((b) => [b.id, b])), [items]);

  return { items, loading, create, update, remove, byId };
}
