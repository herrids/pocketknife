import { useState, useEffect, useCallback } from "react";
import { toast } from "sonner";
import { ApiError } from "@/client";
import type { Question, QuestionCreateInput, QuestionUpdateInput } from "@/client";
import { client } from "@/lib/client";

export function useQuestions() {
  const [questions, setQuestions] = useState<Question[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await client.question.list({ sort: ["-created_at"], limit: 200 });
      setQuestions(result.data);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Laden der Fragen");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = useCallback(async (input: QuestionCreateInput): Promise<Question> => {
    const q = await client.question.create(input);
    setQuestions((prev) => [q, ...prev]);
    toast.success("Frage erstellt");
    return q;
  }, []);

  const update = useCallback(async (id: string, input: QuestionUpdateInput): Promise<Question> => {
    const q = await client.question.update(id, input);
    setQuestions((prev) => prev.map((x) => (x.id === id ? q : x)));
    toast.success("Frage gespeichert");
    return q;
  }, []);

  const remove = useCallback(async (id: string): Promise<void> => {
    await client.question.delete(id);
    setQuestions((prev) => prev.filter((x) => x.id !== id));
    toast.success("Frage gelöscht");
  }, []);

  return { questions, loading, refetch: load, create, update, remove };
}
