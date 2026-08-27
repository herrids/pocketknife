import { useState, useEffect } from "react";
import { Plus, Pencil, Trash2, Star } from "lucide-react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import { client } from "@/lib/client";
import { ApiError } from "@/client";
import type { Faehigkeit, FaehigkeitKategorie, Sprung } from "@/client";
import { cn } from "@/lib/utils";
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

const KATEGORIE_CONFIG: Record<
  FaehigkeitKategorie,
  { label: string; className: string }
> = {
  motorik: {
    label: "Motorik",
    className:
      "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300",
  },
  sprache: {
    label: "Sprache",
    className:
      "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300",
  },
  soziales: {
    label: "Soziales",
    className:
      "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300",
  },
  wahrnehmung: {
    label: "Wahrnehmung",
    className:
      "bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300",
  },
  emotion: {
    label: "Emotion",
    className:
      "bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300",
  },
};

const KATEGORIEN: FaehigkeitKategorie[] = [
  "motorik",
  "sprache",
  "soziales",
  "wahrnehmung",
  "emotion",
];

function isKategorie(v: string): v is FaehigkeitKategorie {
  return KATEGORIEN.includes(v as FaehigkeitKategorie);
}

export default function FaehigkeitView() {
  const [faehigkeiten, setFaehigkeiten] = useState<Faehigkeit[]>([]);
  const [spruenge, setSpruenge] = useState<Sprung[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Faehigkeit | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [sprungId, setSprungId] = useState("");
  const [bezeichnung, setBezeichnung] = useState("");
  const [kategorie, setKategorie] = useState("");
  const [beschreibung, setBeschreibung] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    load();
  }, []);

  async function load() {
    try {
      const [fResult, sResult] = await Promise.all([
        client.faehigkeit.list({ sort: ["kategorie"], limit: 200 }),
        client.sprung.list({ sort: ["nummer"], limit: 100 }),
      ]);
      setFaehigkeiten(fResult.data);
      setSpruenge(sResult.data);
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    } finally {
      setLoading(false);
    }
  }

  function resetForm() {
    setSprungId("");
    setBezeichnung("");
    setKategorie("");
    setBeschreibung("");
    setErrors({});
  }

  function openCreate() {
    setEditing(null);
    resetForm();
    setOpen(true);
  }

  function openEdit(f: Faehigkeit) {
    setEditing(f);
    setSprungId(f.sprung_id);
    setBezeichnung(f.bezeichnung);
    setKategorie(f.kategorie);
    setBeschreibung(f.beschreibung ?? "");
    setErrors({});
    setOpen(true);
  }

  const sprungMap = new Map(spruenge.map((s) => [s.id, s]));

  function validate(): boolean {
    const errs: Record<string, string> = {};
    if (!sprungId) errs.sprungId = "Bitte einen Sprung auswählen";
    if (!bezeichnung.trim()) errs.bezeichnung = "Pflichtfeld";
    if (!isKategorie(kategorie)) errs.kategorie = "Bitte eine Kategorie auswählen";
    setErrors(errs);
    return Object.keys(errs).length === 0;
  }

  async function handleSubmit() {
    if (!validate()) return;
    if (!isKategorie(kategorie)) return;
    setSubmitting(true);
    try {
      const input = {
        sprung_id: sprungId,
        bezeichnung: bezeichnung.trim(),
        kategorie,
        beschreibung: beschreibung.trim() || null,
      };
      if (editing) {
        const updated = await client.faehigkeit.update(editing.id, input);
        setFaehigkeiten((prev) =>
          prev.map((f) => (f.id === updated.id ? updated : f))
        );
        toast.success("Fähigkeit aktualisiert");
      } else {
        const created = await client.faehigkeit.create(input);
        setFaehigkeiten((prev) => [...prev, created]);
        toast.success("Fähigkeit hinzugefügt");
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
      await client.faehigkeit.delete(id);
      setFaehigkeiten((prev) => prev.filter((f) => f.id !== id));
      toast.success("Fähigkeit entfernt");
    } catch (e) {
      if (e instanceof ApiError) toast.error(e.message);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold tracking-tight">Fähigkeiten</h2>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" />
          Hinzufügen
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-24 w-full rounded-xl" />
          ))}
        </div>
      ) : faehigkeiten.length === 0 ? (
        <div className="flex flex-col items-center gap-3 py-20 text-center">
          <div className="rounded-full bg-muted p-5">
            <Star className="h-8 w-8 text-muted-foreground" />
          </div>
          <div>
            <p className="font-medium">Keine Fähigkeiten eingetragen</p>
            <p className="text-sm text-muted-foreground mt-1">
              Trage die Fähigkeiten der einzelnen Sprünge ein.
            </p>
          </div>
          <Button onClick={openCreate} size="sm">
            Erste Fähigkeit anlegen
          </Button>
        </div>
      ) : (
        <AnimatePresence initial={false}>
          {faehigkeiten.map((f) => {
            const sprung = sprungMap.get(f.sprung_id);
            const config = KATEGORIE_CONFIG[f.kategorie];
            return (
              <motion.div
                key={f.id}
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
                        <div className="flex items-center gap-2 flex-wrap mb-1">
                          <span
                            className={cn(
                              "inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium",
                              config.className
                            )}
                          >
                            {config.label}
                          </span>
                          {sprung && (
                            <span className="inline-flex items-center rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs font-medium">
                              Sprung {sprung.nummer}
                            </span>
                          )}
                        </div>
                        <p className="font-semibold leading-tight">{f.bezeichnung}</p>
                      </div>
                      <div className="flex gap-1 shrink-0">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => openEdit(f)}
                          aria-label="Bearbeiten"
                          className="h-8 w-8"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => handleDelete(f.id)}
                          aria-label="Löschen"
                          className="h-8 w-8 text-destructive hover:text-destructive"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </div>
                  </CardHeader>
                  {(f.beschreibung !== null || sprung !== undefined) && (
                    <CardContent className="px-4 pb-4 pt-0">
                      {f.beschreibung !== null && (
                        <p className="text-sm text-muted-foreground line-clamp-2">
                          {f.beschreibung}
                        </p>
                      )}
                      {sprung !== undefined && (
                        <p className="text-xs text-muted-foreground mt-1">
                          {sprung.bezeichnung}
                        </p>
                      )}
                    </CardContent>
                  )}
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
              {editing ? "Fähigkeit bearbeiten" : "Fähigkeit hinzufügen"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-1">
            <div className="space-y-1.5">
              <Label htmlFor="fae-sprung">Sprung *</Label>
              {spruenge.length === 0 ? (
                <p className="text-sm text-muted-foreground rounded-md border border-input bg-muted px-3 py-2">
                  Zuerst einen Sprung anlegen
                </p>
              ) : (
                <Select value={sprungId} onValueChange={setSprungId}>
                  <SelectTrigger id="fae-sprung">
                    <SelectValue placeholder="Sprung auswählen…" />
                  </SelectTrigger>
                  <SelectContent>
                    {spruenge.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        Sprung {s.nummer} – {s.bezeichnung}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              {errors.sprungId && (
                <p className="text-xs text-destructive">{errors.sprungId}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="fae-bezeichnung">Bezeichnung *</Label>
              <Input
                id="fae-bezeichnung"
                value={bezeichnung}
                onChange={(e) => setBezeichnung(e.target.value)}
                placeholder="z. B. Greift nach Gegenständen"
                autoFocus
              />
              {errors.bezeichnung && (
                <p className="text-xs text-destructive">{errors.bezeichnung}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="fae-kategorie">Kategorie *</Label>
              <Select value={kategorie} onValueChange={setKategorie}>
                <SelectTrigger id="fae-kategorie">
                  <SelectValue placeholder="Kategorie auswählen…" />
                </SelectTrigger>
                <SelectContent>
                  {KATEGORIEN.map((k) => (
                    <SelectItem key={k} value={k}>
                      {KATEGORIE_CONFIG[k].label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {errors.kategorie && (
                <p className="text-xs text-destructive">{errors.kategorie}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="fae-beschreibung">Beschreibung</Label>
              <Textarea
                id="fae-beschreibung"
                value={beschreibung}
                onChange={(e) => setBeschreibung(e.target.value)}
                placeholder="Optionale Beschreibung…"
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
