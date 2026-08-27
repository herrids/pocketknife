import { useState, type FormEvent } from "react";
import { Plus, Pencil, Trash2 } from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import { Faehigkeit, FaehigkeitCreateInput, FaehigkeitKategorie, FaehigkeitUpdateInput } from "@/client";
import { useFaehigkeit } from "@/hooks/use-faehigkeit";
import { useSprung } from "@/hooks/use-sprung";
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
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
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
import { KATEGORIE_COLORS, KATEGORIE_LABELS, KATEGORIE_OPTIONS } from "@/lib/constants";

type DialogMode = null | { type: "create" } | { type: "edit"; item: Faehigkeit };

interface FormState {
  sprung_id: string;
  bezeichnung: string;
  kategorie: string;
  beschreibung: string;
}

function emptyForm(): FormState {
  return { sprung_id: "", bezeichnung: "", kategorie: "", beschreibung: "" };
}

function itemToForm(item: Faehigkeit): FormState {
  return {
    sprung_id: item.sprung_id,
    bezeichnung: item.bezeichnung,
    kategorie: item.kategorie,
    beschreibung: item.beschreibung ?? "",
  };
}

export function FaehigkeitView() {
  const { items, loading, create, update, remove } = useFaehigkeit();
  const { items: spruenge, loading: sprungLoading } = useSprung();
  const [mode, setMode] = useState<DialogMode>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const sprungById = new Map(spruenge.map((s) => [s.id, s]));

  function openCreate() {
    setForm(emptyForm());
    setMode({ type: "create" });
  }

  function openEdit(item: Faehigkeit) {
    setForm(itemToForm(item));
    setMode({ type: "edit", item });
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!form.sprung_id || !form.bezeichnung.trim() || !form.kategorie) return;
    setSaving(true);
    const payload: FaehigkeitCreateInput = {
      sprung_id: form.sprung_id,
      bezeichnung: form.bezeichnung.trim(),
      kategorie: form.kategorie as FaehigkeitKategorie,
      beschreibung: form.beschreibung.trim() || null,
    };
    const ok =
      mode?.type === "create"
        ? await create(payload)
        : mode?.type === "edit"
          ? await update(mode.item.id, payload as FaehigkeitUpdateInput)
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
        <h2 className="text-xl font-semibold">Fähigkeiten</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Hinzufügen
        </Button>
      </div>

      {loading || sprungLoading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <div className="flex gap-2 mb-2">
                  <Skeleton className="h-5 w-20 rounded-full" />
                  <Skeleton className="h-5 w-32" />
                </div>
                <Skeleton className="h-4 w-24" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 gap-4 text-center">
          <span className="text-5xl">⭐</span>
          <div>
            <p className="font-medium">Noch keine Fähigkeiten erfasst</p>
            <p className="text-sm text-muted-foreground mt-1">
              Welche Fähigkeiten entwickelt dein Baby?
            </p>
          </div>
          <Button onClick={openCreate} disabled={spruenge.length === 0}>
            <Plus className="h-4 w-4 mr-1.5" />
            Fähigkeit hinzufügen
          </Button>
          {spruenge.length === 0 && (
            <p className="text-xs text-muted-foreground">Bitte erst einen Sprung anlegen.</p>
          )}
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {items.map((item) => {
            const sprung = sprungById.get(item.sprung_id);
            const kat = item.kategorie as FaehigkeitKategorie;
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
                          <span
                            className={cn(
                              "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium",
                              KATEGORIE_COLORS[kat],
                            )}
                          >
                            {KATEGORIE_LABELS[kat]}
                          </span>
                          <span className="font-medium truncate">{item.bezeichnung}</span>
                        </div>
                        {sprung && (
                          <p className="text-xs text-muted-foreground">
                            Sprung #{sprung.nummer} · {sprung.bezeichnung}
                          </p>
                        )}
                        {item.beschreibung && (
                          <p className="text-sm text-muted-foreground mt-1.5 line-clamp-2">
                            {item.beschreibung}
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
        <DialogContent className="max-h-[90dvh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {mode?.type === "edit" ? "Fähigkeit bearbeiten" : "Fähigkeit hinzufügen"}
            </DialogTitle>
            <DialogDescription>
              Fähigkeit {mode?.type === "edit" ? "aktualisieren" : "erfassen"}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="f-sprung">Sprung *</Label>
              <Select
                value={form.sprung_id}
                onValueChange={(v) => setForm((f) => ({ ...f, sprung_id: v }))}
                required
              >
                <SelectTrigger id="f-sprung">
                  <SelectValue placeholder="Sprung auswählen…" />
                </SelectTrigger>
                <SelectContent>
                  {spruenge.map((s) => (
                    <SelectItem key={s.id} value={s.id}>
                      #{s.nummer} · {s.bezeichnung}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="f-bezeichnung">Bezeichnung *</Label>
              <Input
                id="f-bezeichnung"
                value={form.bezeichnung}
                onChange={(e) => setForm((f) => ({ ...f, bezeichnung: e.target.value }))}
                placeholder="z.B. Hebt den Kopf selbstständig"
                required
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="f-kategorie">Kategorie *</Label>
              <Select
                value={form.kategorie}
                onValueChange={(v) => setForm((f) => ({ ...f, kategorie: v }))}
                required
              >
                <SelectTrigger id="f-kategorie">
                  <SelectValue placeholder="Kategorie auswählen…" />
                </SelectTrigger>
                <SelectContent>
                  {KATEGORIE_OPTIONS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {KATEGORIE_LABELS[k]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="f-beschreibung">Beschreibung</Label>
              <Textarea
                id="f-beschreibung"
                value={form.beschreibung}
                onChange={(e) => setForm((f) => ({ ...f, beschreibung: e.target.value }))}
                placeholder="Optionale Beschreibung…"
                rows={3}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setMode(null)}>
                Abbrechen
              </Button>
              <Button
                type="submit"
                disabled={saving || !form.sprung_id || !form.kategorie}
              >
                {saving ? "Speichern…" : "Speichern"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteId !== null} onOpenChange={(open) => !open && setDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Fähigkeit löschen?</DialogTitle>
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
