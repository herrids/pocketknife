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
import { Textarea } from "@/components/ui/textarea";
import type { Bean, BeanCreateInput, BeanUpdateInput } from "@/client";

interface BeanFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bean: Bean | null;
  onCreate: (input: BeanCreateInput) => Promise<Bean | null>;
  onUpdate: (id: string, input: BeanUpdateInput) => Promise<Bean | null>;
}

interface FormState {
  name: string;
  roaster: string;
  roastDate: string;
  notes: string;
}

const EMPTY_FORM: FormState = { name: "", roaster: "", roastDate: "", notes: "" };

function toFormState(bean: Bean | null): FormState {
  if (!bean) return EMPTY_FORM;
  return {
    name: bean.name,
    roaster: bean.roaster ?? "",
    roastDate: bean.roast_date ? bean.roast_date.slice(0, 10) : "",
    notes: bean.notes ?? "",
  };
}

/** Create/edit form for a bean, in a Dialog. Validates against the manifest's constraints before submitting. */
export function BeanFormDialog({ open, onOpenChange, bean, onCreate, onUpdate }: BeanFormDialogProps) {
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [errors, setErrors] = useState<Partial<Record<keyof FormState, string>>>({});
  const [submitting, setSubmitting] = useState(false);
  const isEditing = bean !== null;

  useEffect(() => {
    if (open) {
      setForm(toFormState(bean));
      setErrors({});
    }
  }, [open, bean]);

  function validate(): boolean {
    const next: Partial<Record<keyof FormState, string>> = {};
    const name = form.name.trim();
    if (!name) next.name = "Name is required.";
    else if (name.length > 120) next.name = "Keep it under 120 characters.";
    if (form.roaster.trim().length > 120) next.roaster = "Keep it under 120 characters.";
    if (form.notes.length > 500) next.notes = "Keep it under 500 characters.";
    setErrors(next);
    return Object.keys(next).length === 0;
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    if (!validate()) return;

    setSubmitting(true);
    const payload = {
      name: form.name.trim(),
      roaster: form.roaster.trim() ? form.roaster.trim() : null,
      roast_date: form.roastDate ? new Date(form.roastDate).toISOString() : null,
      notes: form.notes.trim() ? form.notes.trim() : null,
    };
    const result = isEditing && bean ? await onUpdate(bean.id, payload) : await onCreate(payload);
    setSubmitting(false);
    if (result) onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{isEditing ? "Edit bean" : "Add a bean"}</DialogTitle>
            <DialogDescription>
              {isEditing ? "Update this bean's details." : "Track a new bag of beans you're brewing with."}
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-4">
            <div className="grid gap-1.5">
              <Label htmlFor="bean-name">
                Name <span className="text-destructive">*</span>
              </Label>
              <Input
                id="bean-name"
                value={form.name}
                maxLength={120}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder="Ethiopia Guji Natural"
                aria-invalid={Boolean(errors.name)}
              />
              {errors.name && <p className="text-xs text-destructive">{errors.name}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="bean-roaster">Roaster</Label>
              <Input
                id="bean-roaster"
                value={form.roaster}
                maxLength={120}
                onChange={(e) => setForm((f) => ({ ...f, roaster: e.target.value }))}
                placeholder="Square Mile"
                aria-invalid={Boolean(errors.roaster)}
              />
              {errors.roaster && <p className="text-xs text-destructive">{errors.roaster}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="bean-roast-date">Roast date</Label>
              <Input
                id="bean-roast-date"
                type="date"
                value={form.roastDate}
                onChange={(e) => setForm((f) => ({ ...f, roastDate: e.target.value }))}
              />
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="bean-notes">Notes</Label>
              <Textarea
                id="bean-notes"
                value={form.notes}
                maxLength={500}
                rows={3}
                onChange={(e) => setForm((f) => ({ ...f, notes: e.target.value }))}
                placeholder="Tasting notes, brew ideas…"
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
              {isEditing ? "Save changes" : "Add bean"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
