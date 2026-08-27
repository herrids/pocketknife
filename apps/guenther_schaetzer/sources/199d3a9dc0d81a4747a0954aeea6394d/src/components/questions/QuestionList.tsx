import { useState } from "react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import { Plus, Pencil, Trash2, BookOpen, Lightbulb } from "lucide-react";
import { ApiError } from "@/client";
import type { Question, QuestionCategory, QuestionDifficulty } from "@/client";
import { useQuestions } from "@/hooks/useQuestions";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import {
  CATEGORY_LABELS,
  CATEGORY_COLORS,
  CATEGORY_EMOJIS,
  DIFFICULTY_LABELS,
  DIFFICULTY_COLORS,
} from "@/lib/labels";

type FormValues = {
  text: string;
  correct_answer: string;
  unit: string;
  category: string;
  difficulty: string;
  hint: string;
};

function emptyForm(): FormValues {
  return { text: "", correct_answer: "", unit: "", category: "", difficulty: "", hint: "" };
}

function questionToForm(q: Question): FormValues {
  return {
    text: q.text,
    correct_answer: String(q.correct_answer),
    unit: q.unit ?? "",
    category: q.category,
    difficulty: q.difficulty,
    hint: q.hint ?? "",
  };
}

function validate(v: FormValues): Partial<FormValues> {
  const e: Partial<FormValues> = {};
  if (!v.text.trim()) e.text = "Pflichtfeld";
  if (!v.correct_answer.trim()) e.correct_answer = "Pflichtfeld";
  else if (isNaN(Number(v.correct_answer)) || !Number.isInteger(Number(v.correct_answer)))
    e.correct_answer = "Muss eine ganze Zahl sein";
  if (!v.category) e.category = "Bitte wählen";
  if (!v.difficulty) e.difficulty = "Bitte wählen";
  return e;
}

const CATEGORIES = Object.keys(CATEGORY_LABELS) as QuestionCategory[];
const DIFFICULTIES = Object.keys(DIFFICULTY_LABELS) as QuestionDifficulty[];

export default function QuestionList() {
  const { questions, loading, create, update, remove } = useQuestions();
  const [dialogMode, setDialogMode] = useState<"create" | "edit" | null>(null);
  const [editTarget, setEditTarget] = useState<Question | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Question | null>(null);
  const [form, setForm] = useState<FormValues>(emptyForm());
  const [errors, setErrors] = useState<Partial<FormValues>>({});
  const [saving, setSaving] = useState(false);

  function openCreate() {
    setForm(emptyForm());
    setErrors({});
    setEditTarget(null);
    setDialogMode("create");
  }

  function openEdit(q: Question) {
    setForm(questionToForm(q));
    setErrors({});
    setEditTarget(q);
    setDialogMode("edit");
  }

  function setField(field: keyof FormValues, value: string) {
    setForm((f) => ({ ...f, [field]: value }));
    setErrors((e) => ({ ...e, [field]: undefined }));
  }

  async function handleSubmit() {
    const errs = validate(form);
    if (Object.keys(errs).length > 0) {
      setErrors(errs);
      return;
    }
    setSaving(true);
    try {
      const input = {
        text: form.text.trim(),
        correct_answer: parseInt(form.correct_answer, 10),
        category: form.category as QuestionCategory,
        difficulty: form.difficulty as QuestionDifficulty,
        unit: form.unit.trim() || null,
        hint: form.hint.trim() || null,
      };
      if (dialogMode === "edit" && editTarget) {
        await update(editTarget.id, input);
      } else {
        await create(input);
      }
      setDialogMode(null);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Speichern");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    setSaving(true);
    try {
      await remove(deleteTarget.id);
      setDeleteTarget(null);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Löschen");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Fragendatenbank</h2>
          {!loading && (
            <p className="text-sm text-muted-foreground">
              {questions.length} {questions.length === 1 ? "Frage" : "Fragen"}
            </p>
          )}
        </div>
        <Button onClick={openCreate} size="sm" className="gap-1.5">
          <Plus className="h-4 w-4" />
          Neue Frage
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full rounded-xl" />
          ))}
        </div>
      ) : questions.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed py-16 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-muted text-3xl">
            📝
          </div>
          <div>
            <p className="font-medium">Noch keine Fragen</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Erstelle die erste Frage für dein Schätzduell
            </p>
          </div>
          <Button onClick={openCreate} variant="outline" size="sm" className="gap-1.5">
            <Plus className="h-4 w-4" />
            Erste Frage erstellen
          </Button>
        </div>
      ) : (
        <motion.div
          className="space-y-3"
          initial="hidden"
          animate="visible"
          variants={{ visible: { transition: { staggerChildren: 0.04 } } }}
        >
          <AnimatePresence mode="popLayout">
            {questions.map((q) => (
              <motion.div
                key={q.id}
                variants={{
                  hidden: { opacity: 0, y: 8 },
                  visible: { opacity: 1, y: 0, transition: { duration: 0.2 } },
                }}
                exit={{ opacity: 0, y: -8, transition: { duration: 0.15 } }}
                layout
              >
                <Card className="overflow-hidden transition-shadow hover:shadow-md">
                  <CardContent className="p-4">
                    <div className="flex items-start gap-3">
                      <span className="mt-0.5 text-xl leading-none">
                        {CATEGORY_EMOJIS[q.category]}
                      </span>
                      <div className="min-w-0 flex-1 space-y-2">
                        <p className="font-medium leading-snug">{q.text}</p>
                        <div className="flex flex-wrap items-center gap-1.5">
                          <Badge
                            className={cn(
                              "border-0 text-xs font-medium",
                              CATEGORY_COLORS[q.category],
                            )}
                          >
                            {CATEGORY_LABELS[q.category]}
                          </Badge>
                          <Badge
                            className={cn(
                              "border-0 text-xs font-medium",
                              DIFFICULTY_COLORS[q.difficulty],
                            )}
                          >
                            {DIFFICULTY_LABELS[q.difficulty]}
                          </Badge>
                          <span className="text-sm font-semibold text-foreground">
                            {q.correct_answer}
                            {q.unit ? ` ${q.unit}` : ""}
                          </span>
                        </div>
                        {q.hint && (
                          <div className="flex items-start gap-1.5 rounded-lg bg-muted px-2.5 py-1.5">
                            <Lightbulb className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                            <p className="text-xs text-muted-foreground">{q.hint}</p>
                          </div>
                        )}
                      </div>
                      <div className="flex shrink-0 gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8"
                          aria-label="Frage bearbeiten"
                          onClick={() => openEdit(q)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-destructive hover:text-destructive"
                          aria-label="Frage löschen"
                          onClick={() => setDeleteTarget(q)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </AnimatePresence>
        </motion.div>
      )}

      {/* Create / Edit Dialog */}
      <Dialog open={dialogMode !== null} onOpenChange={(o) => !o && setDialogMode(null)}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <BookOpen className="h-5 w-5" />
              {dialogMode === "edit" ? "Frage bearbeiten" : "Neue Frage"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="q-text">Frage *</Label>
              <Textarea
                id="q-text"
                placeholder="z.B. Wie viele Tore schoss Gerd Müller in einer Bundesligasaison?"
                value={form.text}
                onChange={(e) => setField("text", e.target.value)}
                className={cn("resize-none", errors.text && "border-destructive")}
                rows={3}
              />
              {errors.text && <p className="text-xs text-destructive">{errors.text}</p>}
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="q-answer">Richtige Antwort *</Label>
                <Input
                  id="q-answer"
                  type="number"
                  placeholder="z.B. 40"
                  value={form.correct_answer}
                  onChange={(e) => setField("correct_answer", e.target.value)}
                  className={cn(errors.correct_answer && "border-destructive")}
                />
                {errors.correct_answer && (
                  <p className="text-xs text-destructive">{errors.correct_answer}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="q-unit">Einheit</Label>
                <Input
                  id="q-unit"
                  placeholder="z.B. Tore, Mio. €"
                  value={form.unit}
                  onChange={(e) => setField("unit", e.target.value)}
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="q-category">Kategorie *</Label>
                <Select value={form.category} onValueChange={(v) => setField("category", v)}>
                  <SelectTrigger
                    id="q-category"
                    className={cn(errors.category && "border-destructive")}
                  >
                    <SelectValue placeholder="Wählen…" />
                  </SelectTrigger>
                  <SelectContent>
                    {CATEGORIES.map((c) => (
                      <SelectItem key={c} value={c}>
                        {CATEGORY_EMOJIS[c]} {CATEGORY_LABELS[c]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {errors.category && (
                  <p className="text-xs text-destructive">{errors.category}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="q-difficulty">Schwierigkeit *</Label>
                <Select value={form.difficulty} onValueChange={(v) => setField("difficulty", v)}>
                  <SelectTrigger
                    id="q-difficulty"
                    className={cn(errors.difficulty && "border-destructive")}
                  >
                    <SelectValue placeholder="Wählen…" />
                  </SelectTrigger>
                  <SelectContent>
                    {DIFFICULTIES.map((d) => (
                      <SelectItem key={d} value={d}>
                        {DIFFICULTY_LABELS[d]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {errors.difficulty && (
                  <p className="text-xs text-destructive">{errors.difficulty}</p>
                )}
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="q-hint">Tipp (optional)</Label>
              <Textarea
                id="q-hint"
                placeholder="z.B. Denk an die Saison 1971/72…"
                value={form.hint}
                onChange={(e) => setField("hint", e.target.value)}
                className="resize-none"
                rows={2}
              />
            </div>
          </div>
          <DialogFooter className="gap-2 sm:gap-0">
            <DialogClose asChild>
              <Button variant="outline" disabled={saving}>
                Abbrechen
              </Button>
            </DialogClose>
            <Button onClick={handleSubmit} disabled={saving}>
              {saving ? "Speichern…" : dialogMode === "edit" ? "Speichern" : "Erstellen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirmation */}
      <Dialog open={deleteTarget !== null} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Frage löschen?</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            Möchtest du diese Frage wirklich unwiderruflich löschen?
          </p>
          {deleteTarget && (
            <p className="rounded-lg bg-muted px-3 py-2 text-sm italic">
              "{deleteTarget.text}"
            </p>
          )}
          <DialogFooter className="gap-2 sm:gap-0">
            <DialogClose asChild>
              <Button variant="outline" disabled={saving}>
                Abbrechen
              </Button>
            </DialogClose>
            <Button variant="destructive" onClick={handleDelete} disabled={saving}>
              {saving ? "Löschen…" : "Löschen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
