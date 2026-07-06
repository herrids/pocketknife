import { useState, useEffect } from "react";
import { Plus, Pencil, Trash2, BookOpen, Calendar } from "lucide-react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import { client } from "@/lib/client";
import { ApiError } from "@/client";
import type { Tagebuch, Sprung } from "@/client";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
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
} from "@/components/ui/dialog";

const NO_SPRUNG = "__none__";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("de-DE", {
    weekday: "long",
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

export default function TagebuchView() {
  const [eintraege, setEintraege] = useState<Tagebuch[]>([]);
  const [spruenge, setSpruenge] = useState<Sprung[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Tagebuch | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [datum, setDatum] = useState("");
  const [eintrag, setEintrag] = useState("");
  const [sprungId, setSprungId] = useState(NO_SPRUNG);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const [tResult, sResult] = await Promise.all([
        client.tagebuch.list({ sort: ["-datum"], limit: 200 }),
        client.sprung.list({ sort: ["nummer"], limit: 100 }),
      ]);
      setEintraege(tResult.data);
      setSpruenge(sResult.data);
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    } finally {
      setLoading(false);
    }
  }

  function resetForm() {
    setDatum(toDateInput(new Date().toISOString()));
    setEintrag("");
    setSprungId(NO_SPRUNG);
    setErrors({});
  }

  function openCreate() {
    setEditing(null);
    resetForm();
    setOpen(true);
  }

  function openEdit(t: Tagebuch) {
    setEditing(t);
    setDatum(toDateInput(t.datum));
    setEintrag(t.eintrag);
    setSprungId(t.sprung_id ?? NO_SPRUNG);
    setErrors({});
    setOpen(true);
  }

  const sprungMap = new Map(spruenge.map((s) => [s.id, s]));

  function validate(): boolean {
    const errs: Record<string, string> = {};
    if (!datum) errs.datum = "Datum ist erforderlich";
    if (!eintrag.trim()) errs.eintrag = "Eintrag ist erforderlich";
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  async function handleSubmit() {
    if (!validate()) return;
    setSubmitting(true);
    try {
      const input = {
        datum: fromDateInput(datum),
        eintrag: eintrag.trim(),
        sprung_id: sprungId === NO_SPRUNG ? null : sprungId,
      };
      if (editing) {
        const updated = await client.tagebuch.update(editing.id, input);
        setEintraege((prev) =>
          prev
            .map((t) => (t.id === updated.id ? updated : t))
            .sort(
              (a, b) =>
                new Date(b.datum).getTime() - new Date(a.datum).getTime()
            )
        );
        toast.success("Eintrag aktualisiert");
      } else {
        const created = await client.tagebuch.create(input);
        setEintraege((prev) =>
          [created, ...prev].sort(
            (a, b) =>
              new Date(b.datum).getTime() - new Date(a.datum).getTime()
          )
        );
        toast.success("Eintrag gespeichert");
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
      await client.tagebuch.delete(id);
      setEintraege((prev) => prev.filter((t) => t.id !== id));
      toast.success("Eintrag entfernt");
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold tracking-tight">Tagebuch</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Eintrag
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-36 w-full rounded-xl" />
          ))}
        </div>
      ) : eintraege.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-20 text-center">
          <div className="rounded-full bg-muted p-5">
            <BookOpen className="h-8 w-8 text-muted-foreground" />
          </div>
          <div>
            <p className="font-medium">Noch keine Einträge</p>
            <p className="text-sm text-muted-foreground mt-1">
              Halte besondere Momente im Tagebuch fest.
            </p>
          </div>
          <Button onClick={openCreate} size="sm">
            Ersten Eintrag schreiben
          </Button>
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {eintraege.map((t) => {
            const sprung = t.sprung_id ? sprungMap.get(t.sprung_id) : null;
            return (
              <motion.div
                key={t.id}
                initial={{ opacity: 0, y: 12 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, x: -20 }}
                transition={{ duration: 0.2 }}
                className="mb-3"
              >
                <Card>
                  <CardHeader className="p-4 pb-2">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1.5">
                          <Calendar className="h-3 w-3 shrink-0" />
                          <span>{formatDate(t.datum)}</span>
                        </div>
                        {sprung && (
                          <span className="inline-flex items-center rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs font-medium mb-2">
                            Sprung {sprung.nummer} – {sprung.bezeichnung}
                          </span>
                        )}
                      </div>
                      <div className="flex gap-1 shrink-0">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => openEdit(t)}
                          aria-label="Bearbeiten"
                          className="h-8 w-8"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleDelete(t.id)}
                          aria-label="Löschen"
                          className="h-8 w-8 text-destructive hover:text-destructive"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent className="px-4 pb-4 pt-0">
                    <p className="text-sm leading-relaxed whitespace-pre-wrap">
                      {t.eintrag}
                    </p>
                  </CardContent>
                </Card>
              </motion.div>
            );
          })}
        </AnimatePresence>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {editing ? "Eintrag bearbeiten" : "Neuer Tagebucheintrag"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="tgb-datum">Datum *</Label>
              <Input
                id="tgb-datum"
                type="date"
                value={datum}
                onChange={(e) => setDatum(e.target.value)}
              />
              {errors.datum && (
                <p className="text-xs text-destructive">{errors.datum}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="tgb-eintrag">Eintrag *</Label>
              <Textarea
                id="tgb-eintrag"
                value={eintrag}
                onChange={(e) => setEintrag(e.target.value)}
                placeholder="Was ist heute Besonderes passiert?…"
                className="min-h-[140px] resize-none"
                autoFocus
              />
              {errors.eintrag && (
                <p className="text-xs text-destructive">{errors.eintrag}</p>
              )}
            </div>
            {spruenge.length > 0 && (
              <div className="space-y-1.5">
                <Label htmlFor="tgb-sprung">Sprung (optional)</Label>
                <Select value={sprungId} onValueChange={setSprungId}>
                  <SelectTrigger id="tgb-sprung">
                    <SelectValue placeholder="Keinem Sprung zuordnen" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_SPRUNG}>Kein Sprung</SelectItem>
                    {spruenge.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        Sprung {s.nummer} – {s.bezeichnung}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
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
                  : "Eintragen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
