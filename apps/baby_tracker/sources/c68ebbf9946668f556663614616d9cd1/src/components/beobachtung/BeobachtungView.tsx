import { useState, useEffect } from "react";
import { Plus, Pencil, Trash2, Eye, Calendar } from "lucide-react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import { client } from "@/lib/client";
import { ApiError } from "@/client";
import type { Beobachtung, Faehigkeit } from "@/client";
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

export default function BeobachtungView() {
  const [beobachtungen, setBeobachtungen] = useState<Beobachtung[]>([]);
  const [faehigkeiten, setFaehigkeiten] = useState<Faehigkeit[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Beobachtung | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [faehigkeitId, setFaehigkeitId] = useState("");
  const [datum, setDatum] = useState("");
  const [notiz, setNotiz] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const [bResult, fResult] = await Promise.all([
        client.beobachtung.list({ sort: ["-datum"], limit: 200 }),
        client.faehigkeit.list({ sort: ["bezeichnung"], limit: 200 }),
      ]);
      setBeobachtungen(bResult.data);
      setFaehigkeiten(fResult.data);
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    } finally {
      setLoading(false);
    }
  }

  function resetForm() {
    setFaehigkeitId("");
    setDatum(toDateInput(new Date().toISOString()));
    setNotiz("");
    setErrors({});
  }

  function openCreate() {
    setEditing(null);
    resetForm();
    setOpen(true);
  }

  function openEdit(b: Beobachtung) {
    setEditing(b);
    setFaehigkeitId(b.faehigkeit_id);
    setDatum(toDateInput(b.datum));
    setNotiz(b.notiz ?? "");
    setErrors({});
    setOpen(true);
  }

  const faehigkeitMap = new Map(faehigkeiten.map((f) => [f.id, f]));

  function validate(): boolean {
    const errs: Record<string, string> = {};
    if (!faehigkeitId) errs.faehigkeitId = "Bitte eine Fähigkeit auswählen";
    if (!datum) errs.datum = "Datum ist erforderlich";
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  async function handleSubmit() {
    if (!validate()) return;
    setSubmitting(true);
    try {
      const input = {
        faehigkeit_id: faehigkeitId,
        datum: fromDateInput(datum),
        notiz: notiz.trim() || null,
      };
      if (editing) {
        const updated = await client.beobachtung.update(editing.id, input);
        setBeobachtungen((prev) =>
          prev
            .map((b) => (b.id === updated.id ? updated : b))
            .sort(
              (a, b) =>
                new Date(b.datum).getTime() - new Date(a.datum).getTime()
            )
        );
        toast.success("Beobachtung aktualisiert");
      } else {
        const created = await client.beobachtung.create(input);
        setBeobachtungen((prev) =>
          [created, ...prev].sort(
            (a, b) =>
              new Date(b.datum).getTime() - new Date(a.datum).getTime()
          )
        );
        toast.success("Beobachtung gespeichert");
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
      await client.beobachtung.delete(id);
      setBeobachtungen((prev) => prev.filter((b) => b.id !== id));
      toast.success("Beobachtung entfernt");
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold tracking-tight">Beobachtungen</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Eintragen
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[...Array(3)].map((_, i) => (
            <Skeleton key={i} className="h-24 w-full rounded-xl" />
          ))}
        </div>
      ) : beobachtungen.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-20 text-center">
          <div className="rounded-full bg-muted p-5">
            <Eye className="h-8 w-8 text-muted-foreground" />
          </div>
          <div>
            <p className="font-medium">Noch keine Beobachtungen</p>
            <p className="text-sm text-muted-foreground mt-1">
              Notiere, wenn dein Baby eine Fähigkeit zeigt.
            </p>
          </div>
          <Button onClick={openCreate} size="sm">
            Erste Beobachtung eintragen
          </Button>
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {beobachtungen.map((b) => {
            const faehigkeit = faehigkeitMap.get(b.faehigkeit_id);
            return (
              <motion.div
                key={b.id}
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
                        <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                          <Calendar className="h-3 w-3" />
                          <span>{formatDate(b.datum)}</span>
                        </div>
                        <p className="font-semibold leading-tight">
                          {faehigkeit?.bezeichnung ?? "—"}
                        </p>
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
                  {b.notiz && (
                    <CardContent className="px-4 pb-4 pt-0">
                      <p className="text-sm text-muted-foreground">{b.notiz}</p>
                    </CardContent>
                  )}
                </Card>
              </motion.div>
            );
          })}
        </AnimatePresence>
      )}

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editing ? "Beobachtung bearbeiten" : "Beobachtung eintragen"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="beob-datum">Datum *</Label>
              <Input
                id="beob-datum"
                type="date"
                value={datum}
                onChange={(e) => setDatum(e.target.value)}
              />
              {errors.datum && (
                <p className="text-xs text-destructive">{errors.datum}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="beob-faehigkeit">Fähigkeit *</Label>
              {faehigkeiten.length === 0 ? (
                <p className="text-sm text-muted-foreground rounded-md border border-input bg-muted px-3 py-2">
                  Zuerst Fähigkeiten anlegen
                </p>
              ) : (
                <Select value={faehigkeitId} onValueChange={setFaehigkeitId}>
                  <SelectTrigger id="beob-faehigkeit">
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
              )}
              {errors.faehigkeitId && (
                <p className="text-xs text-destructive">{errors.faehigkeitId}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="beob-notiz">Notiz</Label>
              <Textarea
                id="beob-notiz"
                value={notiz}
                onChange={(e) => setNotiz(e.target.value)}
                placeholder="Was hast du beobachtet?…"
                className="min-h-[100px] resize-none"
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
                  : "Eintragen"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
