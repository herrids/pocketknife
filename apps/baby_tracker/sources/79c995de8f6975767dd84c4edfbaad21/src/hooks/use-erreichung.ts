import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiError, Erreichung, ErreichungCreateInput, ErreichungUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useErreichung() {
  const [items, setItems] = useState<Erreichung[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await client.erreichung.list({ sort: ["-datum"] });
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

  const create = useCallback(async (input: ErreichungCreateInput): Promise<boolean> => {
    try {
      const item = await client.erreichung.create(input);
      setItems((prev) => [item, ...prev]);
      toast.success("Erfolg eingetragen");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const update = useCallback(
    async (id: string, input: ErreichungUpdateInput): Promise<boolean> => {
      try {
        const updated = await client.erreichung.update(id, input);
        setItems((prev) => prev.map((e) => (e.id === id ? updated : e)));
        toast.success("Erfolg aktualisiert");
        return true;
      } catch (e) {
        if (e instanceof ApiError) toast.error(e.message);
        return false;
      }
    },
    [],
  );

  const remove = useCallback(async (id: string): Promise<boolean> => {
    try {
      await client.erreichung.delete(id);
      setItems((prev) => prev.filter((e) => e.id !== id));
      toast.success("Erfolg gelöscht");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const byId = useMemo(() => new Map(items.map((e) => [e.id, e])), [items]);

  return { items, loading, create, update, remove, byId };
}
