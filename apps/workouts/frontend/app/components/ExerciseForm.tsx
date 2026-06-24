"use client";

import { useState } from "react";
import { api, ApiError, type Exercise, type ExerciseKind, type ExerciseStation } from "@/lib/api";
import { EXERCISE_KINDS, HYROX_STATIONS, humanize, parseDuration, formatDuration } from "@/lib/format";

// Inline form to append a movement to a workout. `position` is the 1-based
// slot the new row will occupy; `workoutId` is set as the reference so the
// API enforces the cascade relationship.
export default function ExerciseForm({
  workoutId,
  position,
  onAdded,
  onCancel,
}: {
  workoutId: string;
  position: number;
  onAdded: (e: Exercise) => void;
  onCancel: () => void;
}) {
  const [kind, setKind] = useState<ExerciseKind>("strength");
  const [name, setName] = useState("");
  const [station, setStation] = useState<ExerciseStation | "">("");
  const [sets, setSets] = useState("");
  const [reps, setReps] = useState("");
  const [weight, setWeight] = useState("");
  const [distance, setDistance] = useState("");
  const [secondsText, setSecondsText] = useState("");
  const [restText, setRestText] = useState("");
  const [pb, setPb] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const seconds = parseDuration(secondsText);
    if (secondsText.trim() !== "" && seconds === null) {
      setError('Time must look like "4:30" or a number of seconds.');
      return;
    }
    const rest = parseDuration(restText);
    if (restText.trim() !== "" && rest === null) {
      setError('Rest must look like "1:30" or a number of seconds.');
      return;
    }

    setBusy(true);
    try {
      const created = await api.exercise.create({
        workout: workoutId,
        position,
        kind,
        name: emptyToNull(name),
        station: kind === "hyrox_station" ? (station === "" ? null : station) : null,
        sets: numOrNull(sets),
        reps: numOrNull(reps),
        weight_kg: numOrNull(weight),
        distance_m: numOrNull(distance),
        seconds,
        rest_seconds: rest,
        personal_best: pb,
      });
      onAdded(created);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add the movement.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card" onSubmit={submit} style={{ marginTop: 12 }}>
      {error && <div className="error">{error}</div>}
      <div className="grid">
        <div className="field">
          <label htmlFor="ex-kind">Kind</label>
          <select id="ex-kind" value={kind} onChange={(e) => setKind(e.target.value as ExerciseKind)}>
            {EXERCISE_KINDS.map((k) => (
              <option key={k} value={k}>
                {humanize(k)}
              </option>
            ))}
          </select>
        </div>
        {kind === "hyrox_station" ? (
          <div className="field">
            <label htmlFor="ex-station">Station</label>
            <select id="ex-station" value={station} onChange={(e) => setStation(e.target.value as ExerciseStation | "")}>
              <option value="">— pick a station —</option>
              {HYROX_STATIONS.map((s) => (
                <option key={s} value={s}>
                  {humanize(s)}
                </option>
              ))}
            </select>
          </div>
        ) : (
          <div className="field">
            <label htmlFor="ex-name">Name</label>
            <input
              id="ex-name"
              type="text"
              maxLength={120}
              placeholder="e.g. Back squat, 1k row…"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
        )}
        <div className="field">
          <label htmlFor="ex-sets">Sets</label>
          <input id="ex-sets" type="number" min={0} value={sets} onChange={(e) => setSets(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="ex-reps">Reps</label>
          <input id="ex-reps" type="number" min={0} value={reps} onChange={(e) => setReps(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="ex-weight">Weight (kg)</label>
          <input id="ex-weight" type="number" min={0} step="0.5" value={weight} onChange={(e) => setWeight(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="ex-dist">Distance (m)</label>
          <input id="ex-dist" type="number" min={0} value={distance} onChange={(e) => setDistance(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="ex-secs">Time</label>
          <input
            id="ex-secs"
            type="text"
            inputMode="numeric"
            placeholder="4:30"
            value={secondsText}
            onChange={(e) => setSecondsText(e.target.value)}
          />
        </div>
        <div className="field">
          <label htmlFor="ex-rest">Rest</label>
          <input
            id="ex-rest"
            type="text"
            inputMode="numeric"
            placeholder="1:30"
            value={restText}
            onChange={(e) => setRestText(e.target.value)}
          />
        </div>
        <div className="field checkbox">
          <input id="ex-pb" type="checkbox" checked={pb} onChange={(e) => setPb(e.target.checked)} />
          <label htmlFor="ex-pb">Personal best 🏆</label>
        </div>
      </div>
      <div className="actions">
        <button type="submit" className="primary" disabled={busy}>
          {busy ? "Adding…" : `Add movement #${position}`}
        </button>
        <button type="button" className="ghost" onClick={onCancel} disabled={busy}>
          Cancel
        </button>
      </div>
    </form>
  );
}

// re-exported tiny helper kept local to avoid leaking into the API module
export function describeEffort(secondsText: string): string {
  const s = parseDuration(secondsText);
  return s === null ? "" : formatDuration(s);
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
