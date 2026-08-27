import { useState, type FormEvent } from "react";
import { Plus, Pencil, Trash2, BookOpen } from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import { Tagebuch, TagebuchCreateInput, TagebuchUpdateInput } from "@/client";
import { useTagebuch } from "@/hooks/use-tagebuch";
import { useSprung } from "@/hooks/use-sprung";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { dateInputToISO, formatDate, isoToDateInput } from "@/lib/format";

type DialogMode = null | { type: "create" } | { type: "edit"; item: Tagebuch };

interface FormState {
  datum: string;
  eintrag: string;
  sprung_id: string;
}

function emptyForm(): FormState {
  const today = new Date();
  const y = today.getFullYear();
  const m = String(today.getMonth() + 1).padStart(2, "0");
  const d = String(today.getDate()).padStart(2, "0");
  return { datum: `${y}-${m}-${d}`, eintrag: "", sprung_id: "" };
}

function itemToForm(item: Tagebuch): FormState {
  return {
    datum: isoToDateInput(item.datum),
    eintrag: item.eintrag,
    sprung_id: item.sprung_id ?? "",
  };
}

const NONE_VALUE = "__none__";

export function TagebuchView() {
  const { items, loading, create, update, remove } = useTagebuch();
  const { items: spruenge, loading: sprungLoading } = useSprung();
  const [mode, setMode] = useState<DialogMode>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const sprungById = new Map(spruenge.map((s) => [s.id, s]));

  function openCreate() {
    setForm(emptyForm());
    setMode({ type: "create" });
  }

  function openEdit(item: Tagebuch) {
    setForm(itemToForm(item));
    setMode({ type: "edit", item });
  }

  function toggleExpanded(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!form.datum || !form.eintrag.trim()) return;
    setSaving(true);
    const payload: TagebuchCreateInput = {
      datum: dateInputToISO(form.datum),
      eintrag: form.eintrag.trim(),
      sprung_id: form.sprung_id || null,
    };
    const ok =
      mode?.type === "create"
        ? await create(payload)
        : mode?.type === "edit"
          ? await update(mode.item.id, payload as TagebuchUpdateInput)
          : false;
    setSaving(false);
    if (ok) setMode(null);
  }

  async function handleDelete(id: string) {
    const ok = await remove(id);
    if (ok) setDeleteId(null);
  }

  return (
    <div className="px-4 py-6 space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Tagebuch</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Eintrag
        </Button>
      </div>

      {loading || sprungLoading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <Skeleton className="h-4 w-32 mb-3" />
                <Skeleton className="h-4 w-full mb-1.5" />
                <Skeleton className="h-4 w-3/4" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 gap-4 text-center">
          <BookOpen className="h-12 w-12 text-muted-foreground/50" />
          <div>
            <p className="font-medium">Das Tagebuch ist noch leer</p>
            <p className="text-sm text-muted-foreground mt-1">
              Halte besondere Momente und Beobachtungen fest.
            </p>
          </div>
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4 mr-1.5" />
            Ersten Eintrag schreiben
          </Button>
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {items.map((item) => {
            const sprung = item.sprung_id ? sprungById.get(item.sprung_id) : undefined;
            const isExp = expanded.has(item.id);
            const isLong = item.eintrag.length > 180;

            return (
              <motion.div
                key={item.id}
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                transition={{ duration: 0.18 }}
                className="mb-3"
              >
                <Card>
                  <CardContent className="p-4">
                    <div className="flex items-start justify-between gap-2 mb-2">
                      <div className="flex items-center gap-2 flex-wrap min-w-0">
                        <span className="text-sm font-medium text-muted-foreground">
                          {formatDate(item.datum)}
                        </span>
                        {sprung && (
                          <Badge variant="secondary" className="text-xs shrink-0">
                            Sprung #{sprung.nummer}
                          </Badge>
                        )}
                      </div>
                      <div className="flex gap-1 shrink-0">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => openEdit(item)}
                          aria-label="Bearbeiten"
                        >
                          <Pencil className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => setDeleteId(item.id)}
                          aria-label="Löschen"
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </div>
                    </div>
                    <p
                      className={
                        isLong && !isExp
                          ? "text-sm text-foreground line-clamp-4"
                          : "text-sm text-foreground"
                      }
                    >
                      {item.eintrag}
                    </p>
                    {isLong && (
                      <button
                        className="mt-1 text-xs text-primary hover:underline"
                        onClick={() => toggleExpanded(item.id)}
                      >
                        {isExp ? "Weniger anzeigen" : "Mehr anzeigen"}
                      </button>
                    )}
                  </CardContent>
                </Card>
              </motion.div>
            );
          })}
        </AnimatePresence>
      )}

      <Dialog open={mode !== null} onOpenChange={(open) => !open && setMode(null)}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {mode?.type === "edit" ? "Eintrag bearbeiten" : "Neuer Eintrag"}
            </DialogTitle>
            <DialogDescription>
              {mode?.type === "edit" ? "Tagebucheintrag aktualisieren" : "Moment festhalten"}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="t-datum">Datum *</Label>
              <Input
                id="t-datum"
                type="date"
                value={form.datum}
                onChange={(e) => setForm((f) => ({ ...f, datum: e.target.value }))}
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="t-eintrag">Eintrag *</Label>
              <Textarea
                id="t-eintrag"
                value={form.eintrag}
                onChange={(e) => setForm((f) => ({ ...f, eintrag: e.target.value }))}
                placeholder="Was hast du heute erlebt oder beobachtet?"
                rows={5}
                required
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="t-sprung">Sprung (optional)</Label>
              <Select
                value={form.sprung_id || NONE_VALUE}
                onValueChange={(v) =>
                  setForm((f) => ({ ...f, sprung_id: v === NONE_VALUE ? "" : v }))
                }
              >
                <SelectTrigger id="t-sprung">
                  <SelectValue placeholder="Kein Sprung zugeordnet" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE_VALUE}>Kein Sprung</SelectItem>
                  {spruenge.map((s) => (
                    <SelectItem key={s.id} value={s.id}>
                      #{s.nummer} · {s.bezeichnung}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setMode(null)}>
                Abbrechen
              </Button>
              <Button type="submit" disabled={saving || !form.eintrag.trim()}>
                {saving ? "Speichern…" : "Speichern"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteId !== null} onOpenChange={(open) => !open && setDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Eintrag löschen?</DialogTitle>
            <DialogDescription>Diese Aktion kann nicht rückgängig gemacht werden.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteId(null)}>
              Abbrechen
            </Button>
            <Button variant="destructive" onClick={() => deleteId && handleDelete(deleteId)}>
              Löschen
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
