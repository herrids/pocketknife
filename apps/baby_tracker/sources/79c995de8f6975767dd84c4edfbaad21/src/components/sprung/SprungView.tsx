import { useState, type ChangeEvent, type FormEvent } from "react";
import { Plus, Pencil, Trash2 } from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import { Sprung, SprungCreateInput, SprungUpdateInput } from "@/client";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";

type DialogMode = null | { type: "create" } | { type: "edit"; item: Sprung };

interface FormState {
  nummer: string;
  bezeichnung: string;
  thema: string;
  startwoche: string;
  endwoche: string;
  beschreibung: string;
}

function emptyForm(): FormState {
  return { nummer: "", bezeichnung: "", thema: "", startwoche: "", endwoche: "", beschreibung: "" };
}

function itemToForm(item: Sprung): FormState {
  return {
    nummer: String(item.nummer),
    bezeichnung: item.bezeichnung,
    thema: item.thema,
    startwoche: String(item.startwoche),
    endwoche: String(item.endwoche),
    beschreibung: item.beschreibung ?? "",
  };
}

function isValid(f: FormState): boolean {
  return (
    f.nummer.trim() !== "" &&
    f.bezeichnung.trim() !== "" &&
    f.thema.trim() !== "" &&
    f.startwoche.trim() !== "" &&
    f.endwoche.trim() !== ""
  );
}

export function SprungView() {
  const { items, loading, create, update, remove } = useSprung();
  const [mode, setMode] = useState<DialogMode>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  function openCreate() {
    setForm(emptyForm());
    setMode({ type: "create" });
  }

  function openEdit(item: Sprung) {
    setForm(itemToForm(item));
    setMode({ type: "edit", item });
  }

  function field(key: keyof FormState) {
    return (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setForm((f) => ({ ...f, [key]: e.target.value }));
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!isValid(form)) return;
    setSaving(true);
    const payload: SprungCreateInput = {
      nummer: parseInt(form.nummer, 10),
      bezeichnung: form.bezeichnung.trim(),
      thema: form.thema.trim(),
      startwoche: parseInt(form.startwoche, 10),
      endwoche: parseInt(form.endwoche, 10),
      beschreibung: form.beschreibung.trim() || null,
    };
    const ok =
      mode?.type === "create"
        ? await create(payload)
        : mode?.type === "edit"
          ? await update(mode.item.id, payload as SprungUpdateInput)
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
        <h2 className="text-xl font-semibold">Entwicklungssprünge</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Hinzufügen
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <Skeleton className="h-4 w-16 mb-2" />
                <Skeleton className="h-5 w-36 mb-1" />
                <Skeleton className="h-4 w-24" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 gap-4 text-center">
          <span className="text-5xl">🌱</span>
          <div>
            <p className="font-medium">Keine Sprünge angelegt</p>
            <p className="text-sm text-muted-foreground mt-1">
              Trage die Entwicklungssprünge deines Babys ein.
            </p>
          </div>
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4 mr-1.5" />
            Ersten Sprung hinzufügen
          </Button>
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {items.map((item) => (
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
                        <Badge variant="secondary" className="font-mono text-xs">
                          #{item.nummer}
                        </Badge>
                        <span className="font-semibold truncate">{item.bezeichnung}</span>
                      </div>
                      <p className="text-sm text-muted-foreground mb-1">{item.thema}</p>
                      <p className="text-xs text-muted-foreground">
                        Woche {item.startwoche}–{item.endwoche}
                      </p>
                      {item.beschreibung && (
                        <p className="text-sm mt-2 line-clamp-2 text-muted-foreground">
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
          ))}
        </AnimatePresence>
      )}

      <Dialog open={mode !== null} onOpenChange={(open) => !open && setMode(null)}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {mode?.type === "edit" ? "Sprung bearbeiten" : "Sprung hinzufügen"}
            </DialogTitle>
            <DialogDescription>
              Entwicklungssprung {mode?.type === "edit" ? "aktualisieren" : "erfassen"}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4 pt-1">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="s-nummer">Nummer *</Label>
                <Input
                  id="s-nummer"
                  type="number"
                  min="1"
                  value={form.nummer}
                  onChange={field("nummer")}
                  placeholder="1"
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="s-thema">Thema *</Label>
                <Input
                  id="s-thema"
                  value={form.thema}
                  onChange={field("thema")}
                  placeholder="z.B. Muster"
                  required
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="s-bezeichnung">Bezeichnung *</Label>
              <Input
                id="s-bezeichnung"
                value={form.bezeichnung}
                onChange={field("bezeichnung")}
                placeholder="z.B. Die Welt der Muster"
                required
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="s-startwoche">Startwoche *</Label>
                <Input
                  id="s-startwoche"
                  type="number"
                  min="1"
                  value={form.startwoche}
                  onChange={field("startwoche")}
                  placeholder="5"
                  required
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="s-endwoche">Endwoche *</Label>
                <Input
                  id="s-endwoche"
                  type="number"
                  min="1"
                  value={form.endwoche}
                  onChange={field("endwoche")}
                  placeholder="7"
                  required
                />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="s-beschreibung">Beschreibung</Label>
              <Textarea
                id="s-beschreibung"
                value={form.beschreibung}
                onChange={field("beschreibung")}
                placeholder="Optionale Beschreibung…"
                rows={3}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setMode(null)}>
                Abbrechen
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? "Speichern…" : "Speichern"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteId !== null} onOpenChange={(open) => !open && setDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Sprung löschen?</DialogTitle>
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
