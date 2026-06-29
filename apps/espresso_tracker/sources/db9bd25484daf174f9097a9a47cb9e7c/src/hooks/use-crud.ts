import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";

import { ApiError, type ListResult } from "@/client";

/**
 * The shape of a generated entity sub-client that this hook needs. Every
 * Pocketknife entity sub-client (BeanClient, BrewClient, …) satisfies this
 * structurally — callers wrap the real client methods in arrow functions so
 * `this` binding and entity-specific list params stay intact.
 */
export interface CrudOps<TRow, TCreate, TUpdate> {
  list: () => Promise<ListResult<TRow>>;
  create: (input: TCreate) => Promise<TRow>;
  update: (id: string, input: TUpdate) => Promise<TRow>;
  delete: (id: string) => Promise<void>;
}

export interface UseCrudResult<TRow, TCreate, TUpdate> {
  rows: TRow[];
  total: number;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  create: (input: TCreate) => Promise<TRow | null>;
  update: (id: string, input: TUpdate) => Promise<TRow | null>;
  remove: (id: string) => Promise<boolean>;
}

function messageFor(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

function capitalize(value: string): string {
  return value.length === 0 ? value : value.charAt(0).toUpperCase() + value.slice(1);
}

/**
 * A small data layer over a generated CRUD sub-client: owns the list +
 * loading/error state, performs create/update/delete through the client,
 * keeps the in-memory list in sync on success, and raises a toast (success
 * or the ApiError's message) on every write. Every entity view is built on
 * top of this so reads/writes never bypass the client.
 */
export function useCrud<TRow extends { id: string }, TCreate, TUpdate>(
  ops: CrudOps<TRow, TCreate, TUpdate>,
  label: string,
): UseCrudResult<TRow, TCreate, TUpdate> {
  const [rows, setRows] = useState<TRow[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await ops.list();
      setRows(result.data);
      setTotal(result.total);
    } catch (err) {
      setError(messageFor(err, `Could not load ${label}s.`));
    } finally {
      setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [label]);

  useEffect(() => {
    void refresh();
    // Run once on mount; `refresh` always closes over the latest ops.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const create = useCallback(
    async (input: TCreate): Promise<TRow | null> => {
      try {
        const row = await ops.create(input);
        setRows((prev) => [row, ...prev]);
        setTotal((t) => t + 1);
        toast.success(`${capitalize(label)} added.`);
        return row;
      } catch (err) {
        toast.error(messageFor(err, `Could not add ${label}.`));
        return null;
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    },
    [label],
  );

  const update = useCallback(
    async (id: string, input: TUpdate): Promise<TRow | null> => {
      try {
        const row = await ops.update(id, input);
        setRows((prev) => prev.map((r) => (r.id === id ? row : r)));
        toast.success(`${capitalize(label)} updated.`);
        return row;
      } catch (err) {
        toast.error(messageFor(err, `Could not update ${label}.`));
        return null;
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    },
    [label],
  );

  const remove = useCallback(
    async (id: string): Promise<boolean> => {
      try {
        await ops.delete(id);
        setRows((prev) => prev.filter((r) => r.id !== id));
        setTotal((t) => Math.max(0, t - 1));
        toast.success(`${capitalize(label)} deleted.`);
        return true;
      } catch (err) {
        toast.error(messageFor(err, `Could not delete ${label}.`));
        return false;
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    },
    [label],
  );

  return { rows, total, loading, error, refresh, create, update, remove };
}
