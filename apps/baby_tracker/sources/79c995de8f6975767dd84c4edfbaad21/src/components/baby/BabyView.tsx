import { useState, type FormEvent } from "react";
import { Plus, Pencil, Trash2 } from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import { Baby, BabyCreateInput, BabyUpdateInput } from "@/client";
import { useBabies } from "@/hooks/use-babies";
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
import { calcAge, dateInputToISO, formatDate, isoToDateInput } from "@/lib/format";

type DialogMode = null | { type: "create" } | { type: "edit"; item: Baby };

interface FormState {
  name: string;
  geburtsdatum: string;
}

function emptyForm(): FormState {
  return { name: "", geburtsdatum: "" };
}

function itemToForm(item: Baby): FormState {
  return { name: item.name, geburtsdatum: isoToDateInput(item.geburtsdatum) };
}

export function BabyView() {
  const { items, loading, create, update, remove } = useBabies();
  const [mode, setMode] = useState<DialogMode>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  function openCreate() {
    setForm(emptyForm());
    setMode({ type: "create" });
  }

  function openEdit(item: Baby) {
    setForm(itemToForm(item));
    setMode({ type: "edit", item });
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!form.name.trim() || !form.geburtsdatum) return;
    setSaving(true);
    const payload = {
      name: form.name.trim(),
      geburtsdatum: dateInputToISO(form.geburtsdatum),
    };
    const ok =
      mode?.type === "create"
        ? await create(payload as BabyCreateInput)
        : mode?.type === "edit"
          ? await update(mode.item.id, payload as BabyUpdateInput)
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
        <h2 className="text-xl font-semibold">Babies</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Hinzufügen
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[0, 1].map((i) => (
            <Card key={i}>
              <CardContent className="p-4">
                <Skeleton className="h-5 w-28 mb-2" />
                <Skeleton className="h-4 w-44" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 gap-4 text-center">
          <span className="text-5xl">👶</span>
          <div>
            <p className="font-medium">Noch kein Baby angelegt</p>
            <p className="text-sm text-muted-foreground mt-1">
              Füge dein Baby hinzu, um loszulegen.
            </p>
          </div>
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4 mr-1.5" />
            Baby hinzufügen
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
                <CardContent className="p-4 flex items-center gap-4">
                  <div className="flex-1 min-w-0">
                    <p className="font-semibold text-base truncate">{item.name}</p>
                    <p className="text-sm text-muted-foreground">
                      {formatDate(item.geburtsdatum)} &middot; {calcAge(item.geburtsdatum)}
                    </p>
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
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </AnimatePresence>
      )}

      <Dialog open={mode !== null} onOpenChange={(open) => !open && setMode(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {mode?.type === "edit" ? "Baby bearbeiten" : "Baby hinzufügen"}
            </DialogTitle>
            <DialogDescription>
              {mode?.type === "edit" ? "Profil aktualisieren" : "Neues Baby anlegen"}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="baby-name">Name *</Label>
              <Input
                id="baby-name"
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder="z.B. Emma"
                required
                autoFocus
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="baby-geburtsdatum">Geburtsdatum *</Label>
              <Input
                id="baby-geburtsdatum"
                type="date"
                value={form.geburtsdatum}
                onChange={(e) => setForm((f) => ({ ...f, geburtsdatum: e.target.value }))}
                required
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
            <DialogTitle>Baby löschen?</DialogTitle>
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
