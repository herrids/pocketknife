import { useState, useEffect } from "react";
import { Plus, Pencil, Trash2, Heart } from "lucide-react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import { client } from "@/lib/client";
import { ApiError } from "@/client";
import type { Baby } from "@/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("de-DE", {
    day: "2-digit",
    month: "long",
    year: "numeric",
  });
}

function toDateInput(iso: string): string {
  const d = new Date(iso);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function fromDateInput(s: string): string {
  const [y, mo, d] = s.split("-").map(Number);
  return new Date(y, mo - 1, d, 12, 0, 0).toISOString();
}

function getAge(birthdate: string): string {
  const birth = new Date(birthdate);
  const now = new Date();
  const totalMonths =
    (now.getFullYear() - birth.getFullYear()) * 12 +
    (now.getMonth() - birth.getMonth());
  if (totalMonths < 0) return "";
  if (totalMonths < 1) {
    const days = Math.floor(
      (now.getTime() - birth.getTime()) / (1000 * 60 * 60 * 24)
    );
    return `${days} ${days === 1 ? "Tag" : "Tage"} alt`;
  }
  if (totalMonths < 24) {
    return `${totalMonths} ${totalMonths === 1 ? "Monat" : "Monate"} alt`;
  }
  const years = Math.floor(totalMonths / 12);
  const rem = totalMonths % 12;
  return rem === 0 ? `${years} Jahre alt` : `${years} J. ${rem} M. alt`;
}

export default function BabyView() {
  const [babies, setBabies] = useState<Baby[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Baby | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [name, setName] = useState("");
  const [geburtsdatum, setGeburtsdatum] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const result = await client.baby.list({ sort: ["-created_at"] });
      setBabies(result.data);
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    setEditing(null);
    setName("");
    setGeburtsdatum("");
    setErrors({});
    setOpen(true);
  }

  function openEdit(b: Baby) {
    setEditing(b);
    setName(b.name);
    setGeburtsdatum(toDateInput(b.geburtsdatum));
    setErrors({});
    setOpen(true);
  }

  function validate(): boolean {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name ist erforderlich";
    if (!geburtsdatum) errs.geburtsdatum = "Geburtsdatum ist erforderlich";
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  async function handleSubmit() {
    if (!validate()) return;
    setSubmitting(true);
    try {
      const input = {
        name: name.trim(),
        geburtsdatum: fromDateInput(geburtsdatum),
      };
      if (editing) {
        const updated = await client.baby.update(editing.id, input);
        setBabies((prev) =>
          prev.map((b) => (b.id === updated.id ? updated : b))
        );
        toast.success("Baby aktualisiert");
      } else {
        const created = await client.baby.create(input);
        setBabies((prev) => [created, ...prev]);
        toast.success("Baby hinzugefügt");
      }
      setOpen(false);
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    } finally {
      setSubmitting(false);
    }
  }

  async function handleDelete(id: string) {
    try {
      await client.baby.delete(id);
      setBabies((prev) => prev.filter((b) => b.id !== id));
      toast.success("Baby entfernt");
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold tracking-tight">Baby</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Hinzufügen
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          <Skeleton className="h-28 w-full rounded-xl" />
          <Skeleton className="h-28 w-full rounded-xl" />
        </div>
      ) : babies.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-20 text-center">
          <div className="rounded-full bg-muted p-5">
            <Heart className="h-8 w-8 text-muted-foreground" />
          </div>
          <div>
            <p className="font-medium">Noch kein Baby eingetragen</p>
            <p className="text-sm text-muted-foreground mt-1">
              Füge dein Baby hinzu, um loszulegen.
            </p>
          </div>
          <Button onClick={openCreate} size="sm">
            Baby hinzufügen
          </Button>
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {babies.map((b) => (
            <motion.div
              key={b.id}
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={{ duration: 0.2 }}
              className="mb-3"
            >
              <Card>
                <CardHeader className="pb-2 p-4">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-3">
                      <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-2xl shrink-0">
                        👶
                      </div>
                      <div>
                        <p className="font-semibold text-base leading-tight">{b.name}</p>
                        <p className="text-sm text-primary font-medium mt-0.5">
                          {getAge(b.geburtsdatum)}
                        </p>
                      </div>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => openEdit(b)}
                        aria-label="Bearbeiten"
                        className="h-8 w-8"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDelete(b.id)}
                        aria-label="Löschen"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="px-4 pb-4 pt-0">
                  <p className="text-sm text-muted-foreground">
                    Geburtsdatum: {formatDate(b.geburtsdatum)}
                  </p>
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </AnimatePresence>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing ? "Baby bearbeiten" : "Baby hinzufügen"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="baby-name">Name *</Label>
              <Input
                id="baby-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Name des Babys"
                autoFocus
              />
              {errors.name && (
                <p className="text-xs text-destructive">{errors.name}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="baby-birthdate">Geburtsdatum *</Label>
              <Input
                id="baby-birthdate"
                type="date"
                value={geburtsdatum}
                onChange={(e) => setGeburtsdatum(e.target.value)}
              />
              {errors.geburtsdatum && (
                <p className="text-xs text-destructive">{errors.geburtsdatum}</p>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Abbrechen
            </Button>
            <Button onClick={handleSubmit} disabled={submitting}>
              {submitting
                ? "Wird gespeichert…"
                : editing
                  ? "Speichern"
                  : "Hinzufügen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
