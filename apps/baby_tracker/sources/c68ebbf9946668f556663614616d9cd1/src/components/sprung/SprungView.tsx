import { useState, useEffect } from "react";
import { Plus, Pencil, Trash2, Zap } from "lucide-react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import { client } from "@/lib/client";
import { ApiError } from "@/client";
import type { Sprung } from "@/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";

export default function SprungView() {
  const [spruenge, setSpruenge] = useState<Sprung[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Sprung | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [nummer, setNummer] = useState("");
  const [bezeichnung, setBezeichnung] = useState("");
  const [thema, setThema] = useState("");
  const [startwoche, setStartwoche] = useState("");
  const [endwoche, setEndwoche] = useState("");
  const [beschreibung, setBeschreibung] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const result = await client.sprung.list({ sort: ["nummer"], limit: 100 });
      setSpruenge(result.data);
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    } finally {
      setLoading(false);
    }
  }

  function resetForm() {
    setNummer("");
    setBezeichnung("");
    setThema("");
    setStartwoche("");
    setEndwoche("");
    setBeschreibung("");
    setErrors({});
  }

  function openCreate() {
    setEditing(null);
    resetForm();
    setOpen(true);
  }

  function openEdit(s: Sprung) {
    setEditing(s);
    setNummer(String(s.nummer));
    setBezeichnung(s.bezeichnung);
    setThema(s.thema);
    setStartwoche(String(s.startwoche));
    setEndwoche(String(s.endwoche));
    setBeschreibung(s.beschreibung ?? "");
    setErrors({});
    setOpen(true);
  }

  function validate(): boolean {
    const errs: Record<string, string> = {};
    if (!nummer.trim() || isNaN(parseInt(nummer)))
      errs.nummer = "Gültige Zahl erforderlich";
    if (!bezeichnung.trim()) errs.bezeichnung = "Pflichtfeld";
    if (!thema.trim()) errs.thema = "Pflichtfeld";
    if (!startwoche.trim() || isNaN(parseInt(startwoche)))
      errs.startwoche = "Gültige Zahl erforderlich";
    if (!endwoche.trim() || isNaN(parseInt(endwoche)))
      errs.endwoche = "Gültige Zahl erforderlich";
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  async function handleSubmit() {
    if (!validate()) return;
    setSubmitting(true);
    try {
      const input = {
        nummer: parseInt(nummer),
        bezeichnung: bezeichnung.trim(),
        thema: thema.trim(),
        startwoche: parseInt(startwoche),
        endwoche: parseInt(endwoche),
        beschreibung: beschreibung.trim() || null,
      };
      if (editing) {
        const updated = await client.sprung.update(editing.id, input);
        setSpruenge((prev) =>
          prev
            .map((s) => (s.id === updated.id ? updated : s))
            .sort((a, b) => a.nummer - b.nummer)
        );
        toast.success("Sprung aktualisiert");
      } else {
        const created = await client.sprung.create(input);
        setSpruenge((prev) =>
          [...prev, created].sort((a, b) => a.nummer - b.nummer)
        );
        toast.success("Sprung hinzugefügt");
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
      await client.sprung.delete(id);
      setSpruenge((prev) => prev.filter((s) => s.id !== id));
      toast.success("Sprung entfernt");
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold tracking-tight">Entwicklungssprünge</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Hinzufügen
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-32 w-full rounded-xl" />
          ))}
        </div>
      ) : spruenge.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-20 text-center">
          <div className="rounded-full bg-muted p-5">
            <Zap className="h-8 w-8 text-muted-foreground" />
          </div>
          <div>
            <p className="font-medium">Keine Sprünge vorhanden</p>
            <p className="text-sm text-muted-foreground mt-1">
              Trage die Entwicklungssprünge ein.
            </p>
          </div>
          <Button onClick={openCreate} size="sm">
            Ersten Sprung anlegen
          </Button>
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {spruenge.map((s) => (
            <motion.div
              key={s.id}
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, x: -20 }}
              transition={{ duration: 0.2 }}
              className="mb-3"
            >
              <Card>
                <CardHeader className="p-4 pb-2">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-start gap-3">
                      <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary text-primary-foreground text-sm font-bold shrink-0">
                        {s.nummer}
                      </div>
                      <div className="min-w-0">
                        <p className="font-semibold leading-tight">{s.bezeichnung}</p>
                        <p className="text-sm text-muted-foreground mt-0.5 italic">
                          {s.thema}
                        </p>
                      </div>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => openEdit(s)}
                        aria-label="Bearbeiten"
                        className="h-8 w-8"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleDelete(s.id)}
                        aria-label="Löschen"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="px-4 pb-4 pt-2">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="inline-flex items-center rounded-full bg-primary/10 text-primary px-2.5 py-0.5 text-xs font-medium">
                      Woche {s.startwoche}–{s.endwoche}
                    </span>
                  </div>
                  {s.beschreibung && (
                    <p className="text-sm text-muted-foreground mt-2 line-clamp-2">
                      {s.beschreibung}
                    </p>
                  )}
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </AnimatePresence>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {editing ? "Sprung bearbeiten" : "Sprung hinzufügen"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-1">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="sp-nummer">Nummer *</Label>
                <Input
                  id="sp-nummer"
                  type="number"
                  min={1}
                  value={nummer}
                  onChange={(e) => setNummer(e.target.value)}
                  placeholder="z. B. 1"
                  autoFocus
                />
                {errors.nummer && (
                  <p className="text-xs text-destructive">{errors.nummer}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="sp-thema">Thema *</Label>
                <Input
                  id="sp-thema"
                  value={thema}
                  onChange={(e) => setThema(e.target.value)}
                  placeholder="z. B. Muster"
                />
                {errors.thema && (
                  <p className="text-xs text-destructive">{errors.thema}</p>
                )}
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="sp-bezeichnung">Bezeichnung *</Label>
              <Input
                id="sp-bezeichnung"
                value={bezeichnung}
                onChange={(e) => setBezeichnung(e.target.value)}
                placeholder="z. B. Die Welt der Muster"
              />
              {errors.bezeichnung && (
                <p className="text-xs text-destructive">{errors.bezeichnung}</p>
              )}
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <Label htmlFor="sp-startwoche">Startwoche *</Label>
                <Input
                  id="sp-startwoche"
                  type="number"
                  min={0}
                  value={startwoche}
                  onChange={(e) => setStartwoche(e.target.value)}
                  placeholder="z. B. 5"
                />
                {errors.startwoche && (
                  <p className="text-xs text-destructive">{errors.startwoche}</p>
                )}
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="sp-endwoche">Endwoche *</Label>
                <Input
                  id="sp-endwoche"
                  type="number"
                  min={0}
                  value={endwoche}
                  onChange={(e) => setEndwoche(e.target.value)}
                  placeholder="z. B. 7"
                />
                {errors.endwoche && (
                  <p className="text-xs text-destructive">{errors.endwoche}</p>
                )}
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="sp-beschreibung">Beschreibung</Label>
              <Textarea
                id="sp-beschreibung"
                value={beschreibung}
                onChange={(e) => setBeschreibung(e.target.value)}
                placeholder="Optionale Beschreibung des Sprungs…"
                className="min-h-[80px] resize-none"
              />
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
