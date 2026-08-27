import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiError, Faehigkeit, FaehigkeitCreateInput, FaehigkeitUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useFaehigkeit() {
  const [items, setItems] = useState<Faehigkeit[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await client.faehigkeit.list({ sort: ["bezeichnung"] });
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

  const create = useCallback(async (input: FaehigkeitCreateInput): Promise<boolean> => {
    try {
      const item = await client.faehigkeit.create(input);
      setItems((prev) =>
        [...prev, item].sort((a, b) => a.bezeichnung.localeCompare(b.bezeichnung)),
      );
      toast.success("Fähigkeit hinzugefügt");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const update = useCallback(
    async (id: string, input: FaehigkeitUpdateInput): Promise<boolean> => {
      try {
        const updated = await client.faehigkeit.update(id, input);
        setItems((prev) =>
          prev
            .map((f) => (f.id === id ? updated : f))
            .sort((a, b) => a.bezeichnung.localeCompare(b.bezeichnung)),
        );
        toast.success("Fähigkeit aktualisiert");
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
      await client.faehigkeit.delete(id);
      setItems((prev) => prev.filter((f) => f.id !== id));
      toast.success("Fähigkeit gelöscht");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const byId = useMemo(() => new Map(items.map((f) => [f.id, f])), [items]);

  return { items, loading, create, update, remove, byId };
}
