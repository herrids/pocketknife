import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiError, Tagebuch, TagebuchCreateInput, TagebuchUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useTagebuch() {
  const [items, setItems] = useState<Tagebuch[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await client.tagebuch.list({ sort: ["-datum"] });
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

  const create = useCallback(async (input: TagebuchCreateInput): Promise<boolean> => {
    try {
      const item = await client.tagebuch.create(input);
      setItems((prev) => [item, ...prev]);
      toast.success("Eintrag gespeichert");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const update = useCallback(async (id: string, input: TagebuchUpdateInput): Promise<boolean> => {
    try {
      const updated = await client.tagebuch.update(id, input);
      setItems((prev) => prev.map((t) => (t.id === id ? updated : t)));
      toast.success("Eintrag aktualisiert");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const remove = useCallback(async (id: string): Promise<boolean> => {
    try {
      await client.tagebuch.delete(id);
      setItems((prev) => prev.filter((t) => t.id !== id));
      toast.success("Eintrag gelöscht");
      return true;
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
      return false;
    }
  }, []);

  const byId = useMemo(() => new Map(items.map((t) => [t.id, t])), [items]);

  return { items, loading, create, update, remove, byId };
}
