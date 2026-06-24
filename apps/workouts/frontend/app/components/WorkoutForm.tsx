"use client";

import { useState } from "react";
import { api, ApiError, type Workout, type WorkoutType } from "@/lib/api";
import {
  WORKOUT_TYPES,
  humanize,
  isoToLocalInput,
  localToISO,
  parseDuration,
  formatDuration,
  todayLocalInput,
} from "@/lib/format";

// Create-or-edit form for a workout. When `workout` is provided it edits in
// place (PATCH); otherwise it creates (POST). On success it hands the saved
// row back to the parent so the list/detail can update without a refetch.
export default function WorkoutForm({
  workout,
  onSaved,
  onCancel,
}: {
  workout?: Workout;
  onSaved: (w: Workout) => void;
  onCancel: () => void;
}) {
  const editing = !!workout;
  const [date, setDate] = useState(editing ? isoToLocalInput(workout!.date) : todayLocalInput());
  const [title, setTitle] = useState(workout?.title ?? "");
  const [type, setType] = useState<WorkoutType>(workout?.type ?? "mixed");
  const [totalText, setTotalText] = useState(formatDurationEmpty(workout?.total_seconds));
  const [rpe, setRpe] = useState(workout?.rpe != null ? String(workout!.rpe) : "");
  const [bodyweight, setBodyweight] = useState(workout?.bodyweight_kg != null ? String(workout!.bodyweight_kg) : "");
  const [completed, setCompleted] = useState(workout?.completed ?? false);
  const [notes, setNotes] = useState(workout?.notes ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const iso = localToISO(date);
    if (!iso) {
      setError("A valid date is required.");
      return;
    }
    const total = parseDuration(totalText);
    if (totalText.trim() !== "" && total === null) {
      setError('Total time must look like "1:05:30", "45:00" or a number of seconds.');
      return;
    }

    const payload = {
      date: iso,
      title: emptyToNull(title),
      type,
      total_seconds: total,
      rpe: numOrNull(rpe),
      bodyweight_kg: numOrNull(bodyweight),
      completed,
      notes: emptyToNull(notes),
    };

    setBusy(true);
    try {
      const saved = editing
        ? await api.workout.update(workout!.id, payload)
        : await api.workout.create(payload);
      onSaved(saved);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not save the workout.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card" onSubmit={submit}>
      {error && <div className="error">{error}</div>}
      <div className="grid">
        <div className="field">
          <label htmlFor="wo-date">Date</label>
          <input id="wo-date" type="datetime-local" value={date} onChange={(e) => setDate(e.target.value)} required />
        </div>
        <div className="field">
          <label htmlFor="wo-type">Type</label>
          <select id="wo-type" value={type} onChange={(e) => setType(e.target.value as WorkoutType)}>
            {WORKOUT_TYPES.map((t) => (
              <option key={t} value={t}>
                {humanize(t)}
              </option>
            ))}
          </select>
        </div>
        <div className="field full">
          <label htmlFor="wo-title">Title</label>
          <input
            id="wo-title"
            type="text"
            maxLength={120}
            placeholder="e.g. HYROX simulation, Push day…"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="wo-total">Total time</label>
          <input
            id="wo-total"
            type="text"
            inputMode="numeric"
            placeholder="1:05:30"
            value={totalText}
            onChange={(e) => setTotalText(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="wo-rpe">RPE (1–10)</label>
          <input id="wo-rpe" type="number" min={1} max={10} value={rpe} onChange={(e) => setRpe(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="wo-bw">Bodyweight (kg)</label>
          <input
            id="wo-bw"
            type="number"
            min={0}
            step="0.1"
            value={bodyweight}
            onChange={(e) => setBodyweight(e.target.value)}
          />
        </div>
        <div className="field checkbox">
          <input id="wo-done" type="checkbox" checked={completed} onChange={(e) => setCompleted(e.target.checked)} />
          <label htmlFor="wo-done">Completed</label>
        </div>
        <div className="field full">
          <label htmlFor="wo-notes">Notes</label>
          <textarea
            id="wo-notes"
            maxLength={1000}
            placeholder="How did it feel? Splits, conditions, anything to remember…"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
          />
        </div>
      </div>
      <div className="actions">
        <button type="submit" className="primary" disabled={busy}>
          {busy ? "Saving…" : editing ? "Save changes" : "Add workout"}
        </button>
        <button type="button" className="ghost" onClick={onCancel} disabled={busy}>
          Cancel
        </button>
      </div>
    </form>
  );
}

function formatDurationEmpty(seconds: number | null | undefined): string {
  if (seconds == null || seconds <= 0) return "";
  return formatDuration(seconds);
}

function emptyToNull(s: string): string | null {
  const t = s.trim();
  return t === "" ? null : t;
}

function numOrNull(s: string): number | null {
  const t = s.trim();
  if (t === "") return null;
  const n = Number(t);
  return Number.isFinite(n) ? n : null;
}
