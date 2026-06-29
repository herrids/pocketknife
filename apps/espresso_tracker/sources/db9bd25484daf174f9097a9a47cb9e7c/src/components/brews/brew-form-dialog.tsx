import { useEffect, useState, type FormEvent } from "react";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { StarRating } from "@/components/brews/star-rating";
import { humanize } from "@/lib/format";
import type { Bean, Brew, BrewCreateInput, BrewMethod, BrewUpdateInput } from "@/client";

const METHODS: BrewMethod[] = ["espresso", "aeropress"];
const NO_BEAN = "__none__";

interface BrewFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  brew: Brew | null;
  beans: Bean[];
  onCreate: (input: BrewCreateInput) => Promise<Brew | null>;
  onUpdate: (id: string, input: BrewUpdateInput) => Promise<Brew | null>;
}

interface FormState {
  method: BrewMethod;
  beanId: string;
  doseG: string;
  outputG: string;
  brewTimeS: string;
  grindSetting: string;
  waterTempC: string;
  inverted: boolean;
  rating: number | null;
  notes: string;
}

const EMPTY_FORM: FormState = {
  method: "espresso",
  beanId: NO_BEAN,
  doseG: "",
  outputG: "",
  brewTimeS: "",
  grindSetting: "",
  waterTempC: "",
  inverted: false,
  rating: null,
  notes: "",
};

function toFormState(brew: Brew | null): FormState {
  if (!brew) return EMPTY_FORM;
  return {
    method: brew.method,
    beanId: brew.bean ?? NO_BEAN,
    doseG: String(brew.dose_g),
    outputG: brew.output_g !== null ? String(brew.output_g) : "",
    brewTimeS: brew.brew_time_s !== null ? String(brew.brew_time_s) : "",
    grindSetting: brew.grind_setting ?? "",
    waterTempC: brew.water_temp_c !== null ? String(brew.water_temp_c) : "",
    inverted: brew.inverted ?? false,
    rating: brew.rating,
    notes: brew.notes ?? "",
  };
}

/** Create/edit form for a brew, in a Dialog. Mirrors every constraint the manifest declares (min/max, enum, required). */
export function BrewFormDialog({ open, onOpenChange, brew, beans, onCreate, onUpdate }: BrewFormDialogProps) {
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [errors, setErrors] = useState<Partial<Record<string, string>>>({});
  const [submitting, setSubmitting] = useState(false);
  const isEditing = brew !== null;

  useEffect(() => {
    if (open) {
      setForm(toFormState(brew));
      setErrors({});
    }
  }, [open, brew]);

  function validate(): { valid: boolean; payload: BrewCreateInput } {
    const next: Partial<Record<string, string>> = {};

    const dose = Number.parseFloat(form.doseG);
    if (form.doseG.trim() === "" || Number.isNaN(dose)) next.doseG = "Dose is required.";
    else if (dose < 0 || dose > 50) next.doseG = "Must be between 0 and 50 g.";

    let output: number | null = null;
    if (form.outputG.trim() !== "") {
      output = Number.parseFloat(form.outputG);
      if (Number.isNaN(output) || output < 0 || output > 500) next.outputG = "Must be between 0 and 500 g.";
    }

    let brewTime: number | null = null;
    if (form.brewTimeS.trim() !== "") {
      brewTime = Number.parseInt(form.brewTimeS, 10);
      if (Number.isNaN(brewTime) || brewTime < 0 || brewTime > 300) next.brewTimeS = "Must be between 0 and 300 s.";
    }

    let waterTemp: number | null = null;
    if (form.waterTempC.trim() !== "") {
      waterTemp = Number.parseFloat(form.waterTempC);
      if (Number.isNaN(waterTemp) || waterTemp < 0 || waterTemp > 100) next.waterTempC = "Must be between 0 and 100°C.";
    }

    if (form.grindSetting.trim().length > 50) next.grindSetting = "Keep it under 50 characters.";
    if (form.notes.length > 500) next.notes = "Keep it under 500 characters.";

    setErrors(next);

    return {
      valid: Object.keys(next).length === 0,
      payload: {
        method: form.method,
        bean: form.beanId === NO_BEAN ? null : form.beanId,
        dose_g: Number.isNaN(dose) ? 0 : dose,
        output_g: output,
        brew_time_s: brewTime,
        grind_setting: form.grindSetting.trim() ? form.grindSetting.trim() : null,
        water_temp_c: waterTemp,
        inverted: form.method === "aeropress" ? form.inverted : false,
        rating: form.rating,
        notes: form.notes.trim() ? form.notes.trim() : null,
      },
    };
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const { valid, payload } = validate();
    if (!valid) return;

    setSubmitting(true);
    const result = isEditing && brew ? await onUpdate(brew.id, payload) : await onCreate(payload);
    setSubmitting(false);
    if (result) onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{isEditing ? "Edit brew" : "Log a brew"}</DialogTitle>
            <DialogDescription>
              {isEditing ? "Update this brew's recipe and notes." : "Record the recipe for this cup."}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="brew-method">Method</Label>
                <Select
                  value={form.method}
                  onValueChange={(value) => setForm((f) => ({ ...f, method: value as BrewMethod }))}
                >
                  <SelectTrigger id="brew-method">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {METHODS.map((method) => (
                      <SelectItem key={method} value={method}>
                        {humanize(method)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="brew-bean">Bean</Label>
                <Select value={form.beanId} onValueChange={(value) => setForm((f) => ({ ...f, beanId: value }))}>
                  <SelectTrigger id="brew-bean">
                    <SelectValue placeholder="No bean" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NO_BEAN}>No bean</SelectItem>
                    {beans.map((bean) => (
                      <SelectItem key={bean.id} value={bean.id}>
                        {bean.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="brew-dose">
                  Dose (g) <span className="text-destructive">*</span>
                </Label>
                <Input
                  id="brew-dose"
                  type="number"
                  inputMode="decimal"
                  step="0.1"
                  min={0}
                  max={50}
                  value={form.doseG}
                  onChange={(e) => setForm((f) => ({ ...f, doseG: e.target.value }))}
                  aria-invalid={Boolean(errors.doseG)}
                />
                {errors.doseG && <p className="text-xs text-destructive">{errors.doseG}</p>}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="brew-output">Yield (g)</Label>
                <Input
                  id="brew-output"
                  type="number"
                  inputMode="decimal"
                  step="0.1"
                  min={0}
                  max={500}
                  value={form.outputG}
                  onChange={(e) => setForm((f) => ({ ...f, outputG: e.target.value }))}
                  aria-invalid={Boolean(errors.outputG)}
                />
                {errors.outputG && <p className="text-xs text-destructive">{errors.outputG}</p>}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label htmlFor="brew-time">Brew time (s)</Label>
                <Input
                  id="brew-time"
                  type="number"
                  inputMode="numeric"
                  step="1"
                  min={0}
                  max={300}
                  value={form.brewTimeS}
                  onChange={(e) => setForm((f) => ({ ...f, brewTimeS: e.target.value }))}
                  aria-invalid={Boolean(errors.brewTimeS)}
                />
                {errors.brewTimeS && <p className="text-xs text-destructive">{errors.brewTimeS}</p>}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="brew-temp">Water temp (°C)</Label>
                <Input
                  id="brew-temp"
                  type="number"
                  inputMode="decimal"
                  step="0.5"
                  min={0}
                  max={100}
                  value={form.waterTempC}
                  onChange={(e) => setForm((f) => ({ ...f, waterTempC: e.target.value }))}
                  aria-invalid={Boolean(errors.waterTempC)}
                />
                {errors.waterTempC && <p className="text-xs text-destructive">{errors.waterTempC}</p>}
              </div>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="brew-grind">Grind setting</Label>
              <Input
                id="brew-grind"
                value={form.grindSetting}
                maxLength={50}
                onChange={(e) => setForm((f) => ({ ...f, grindSetting: e.target.value }))}
                placeholder="e.g. 18 clicks"
                aria-invalid={Boolean(errors.grindSetting)}
              />
              {errors.grindSetting && <p className="text-xs text-destructive">{errors.grindSetting}</p>}
            </div>

            {form.method === "aeropress" && (
              <div className="grid gap-1.5">
                <Label>Brew orientation</Label>
                <div className="inline-flex rounded-md border p-1">
                  <Button
                    type="button"
                    size="sm"
                    variant={!form.inverted ? "default" : "ghost"}
                    onClick={() => setForm((f) => ({ ...f, inverted: false }))}
                    className="flex-1"
                  >
                    Standard
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant={form.inverted ? "default" : "ghost"}
                    onClick={() => setForm((f) => ({ ...f, inverted: true }))}
                    className="flex-1"
                  >
                    Inverted
                  </Button>
                </div>
              </div>
            )}

            <div className="grid gap-1.5">
              <Label>Rating</Label>
              <StarRating value={form.rating} onChange={(value) => setForm((f) => ({ ...f, rating: value }))} />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="brew-notes">Notes</Label>
              <Textarea
                id="brew-notes"
                value={form.notes}
                maxLength={500}
                rows={3}
                onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))}
                placeholder="Tasting notes, adjustments for next time…"
                aria-invalid={Boolean(errors.notes)}
              />
              {errors.notes && <p className="text-xs text-destructive">{errors.notes}</p>}
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting && <Loader2 className="h-4 w-4 animate-spin" />}
              {isEditing ? "Save changes" : "Log brew"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
