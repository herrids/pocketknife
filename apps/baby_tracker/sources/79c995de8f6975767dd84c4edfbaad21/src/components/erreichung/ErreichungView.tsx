import { useState, type FormEvent } from "react";
import { Plus, Pencil, Trash2 } from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import { Erreichung, ErreichungCreateInput, ErreichungUpdateInput, FaehigkeitKategorie } from "@/client";
import { useErreichung } from "@/hooks/use-erreichung";
import { useFaehigkeit } from "@/hooks/use-faehigkeit";
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
import { cn } from "@/lib/utils";
import { KATEGORIE_COLORS, KATEGORIE_LABELS } from "@/lib/constants";
import { dateInputToISO, formatDate, isoToDateInput } from "@/lib/format";

type DialogMode = null | { type: "create" } | { type: "edit"; item: Erreichung };

interface FormState {
  faehigkeit_id: string;
  datum: string;
  notiz: string;
}

function emptyForm(): FormState {
  const today = new Date();
  const y = today.getFullYear();
  const m = String(today.getMonth() + 1).padStart(2, "0");
  const d = String(today.getDate()).padStart(2, "0");
  return { faehigkeit_id: "", datum: `${y}-${m}-${d}`, notiz: "" };
}

function itemToForm(item: Erreichung): FormState {
  return {
    faehigkeit_id: item.faehigkeit_id,
    datum: isoToDateInput(item.datum),
    notiz: item.notiz ?? "",
  };
}

export function ErreichungView() {
  const { items, loading, create, update, remove } = useErreichung();
  const { items: faehigkeiten, loading: faehigkeitLoading } = useFaehigkeit();
  const [mode, setMode] = useState<DialogMode>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const faehigkeitById = new Map(faehigkeiten.map((f) => [f.id, f]));

  function openCreate() {
    setForm(emptyForm());
    setMode({ type: "create" });
  }

  function openEdit(item: Erreichung) {
    setForm(itemToForm(item));
    setMode({ type: "edit", item });
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!form.faehigkeit_id || !form.datum) return;
    setSaving(true);
    const payload: ErreichungCreateInput = {
      faehigkeit_id: form.faehigkeit_id,
      datum: dateInputToISO(form.datum),
      notiz: form.notiz.trim() || null,
    };
    const ok =
      mode?.type === "create"
        ? await create(payload)
        : mode?.type === "edit"
          ? await update(mode.item.id, payload as ErreichungUpdateInput)
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
        <h2 className="text-xl font-semibold">Erfolge</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Eintragen
        </Button>
      </div>

      {loading || faehigkeitLoading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <div className="flex gap-2 mb-2">
                  <Skeleton className="h-5 w-20 rounded-full" />
                  <Skeleton className="h-5 w-40" />
                </div>
                <Skeleton className="h-4 w-28" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 gap-4 text-center">
          <span className="text-5xl">🎉</span>
          <div>
            <p className="font-medium">Noch keine Erfolge eingetragen</p>
            <p className="text-sm text-muted-foreground mt-1">
              Wann hat dein Baby eine neue Fähigkeit gezeigt?
            </p>
          </div>
          <Button onClick={openCreate} disabled={faehigkeiten.length === 0}>
            <Plus className="h-4 w-4 mr-1.5" />
            Ersten Erfolg eintragen
          </Button>
          {faehigkeiten.length === 0 && (
            <p className="text-xs text-muted-foreground">Bitte erst eine Fähigkeit anlegen.</p>
          )}
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {items.map((item) => {
            const faehigkeit = faehigkeitById.get(item.faehigkeit_id);
            const kat = faehigkeit?.kategorie as FaehigkeitKategorie | undefined;
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
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap mb-1">
                          {kat && (
                            <span
                              className={cn(
                                "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium shrink-0",
                                KATEGORIE_COLORS[kat],
                              )}
                            >
                              {KATEGORIE_LABELS[kat]}
                            </span>
                          )}
                          <span className="font-medium truncate">
                            {faehigkeit?.bezeichnung ?? item.faehigkeit_id}
                          </span>
                        </div>
                        <p className="text-xs text-muted-foreground">{formatDate(item.datum)}</p>
                        {item.notiz && (
                          <p className="text-sm text-muted-foreground mt-1.5 line-clamp-2">
                            {item.notiz}
                          </p>
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
                  </CardContent>
                </Card>
              </motion.div>
            );
          })}
        </AnimatePresence>
      )}

      <Dialog open={mode !== null} onOpenChange={(open) => !open && setMode(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {mode?.type === "edit" ? "Erfolg bearbeiten" : "Erfolg eintragen"}
            </DialogTitle>
            <DialogDescription>
              Wann hat dein Baby diese Fähigkeit gezeigt?
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="e-faehigkeit">Fähigkeit *</Label>
              <Select
                value={form.faehigkeit_id}
                onValueChange={(v) => setForm((f) => ({ ...f, faehigkeit_id: v }))}
                required
              >
                <SelectTrigger id="e-faehigkeit">
                  <SelectValue placeholder="Fähigkeit auswählen…" />
                </SelectTrigger>
                <SelectContent>
                  {faehigkeiten.map((f) => (
                    <SelectItem key={f.id} value={f.id}>
                      {f.bezeichnung}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="e-datum">Datum *</Label>
              <Input
                id="e-datum"
                type="date"
                value={form.datum}
                onChange={(e) => setForm((f) => ({ ...f, datum: e.target.value }))}
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="e-notiz">Notiz</Label>
              <Textarea
                id="e-notiz"
                value={form.notiz}
                onChange={(e) => setForm((f) => ({ ...f, notiz: e.target.value }))}
                placeholder="Was hast du beobachtet? (optional)"
                rows={3}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setMode(null)}>
                Abbrechen
              </Button>
              <Button type="submit" disabled={saving || !form.faehigkeit_id}>
                {saving ? "Speichern…" : "Speichern"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteId !== null} onOpenChange={(open) => !open && setDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Erfolg löschen?</DialogTitle>
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
