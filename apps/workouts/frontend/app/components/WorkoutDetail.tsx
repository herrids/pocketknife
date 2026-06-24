"use client";

import { useEffect, useState } from "react";
import { api, ApiError, type Exercise, type Workout } from "@/lib/api";
import { formatDate, formatDuration, humanize, typeEmoji } from "@/lib/format";
import WorkoutForm from "./WorkoutForm";
import ExerciseForm from "./ExerciseForm";

// Full view of one workout: editable header + the ordered list of movements.
// Exercises are fetched filtered by the workout reference and sorted by their
// stored position so the session reads top-to-bottom as performed.
export default function WorkoutDetail({
  workout: initial,
  onBack,
  onChanged,
  onDeleted,
}: {
  workout: Workout;
  onBack: () => void;
  onChanged: (w: Workout) => void;
  onDeleted: (id: string) => void;
}) {
  const [workout, setWorkout] = useState(initial);
  const [editing, setEditing] = useState(false);
  const [exercises, setExercises] = useState<Exercise[] | null>(null);
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api.exercise
      .list({ filter: [["workout", "eq", workout.id]], sort: ["position", "created_at"], limit: 200 })
      .then((res) => live && setExercises(res.data))
      .catch((err) => live && setError(err instanceof ApiError ? err.message : "Could not load movements."));
    return () => {
      live = false;
    };
  }, [workout.id]);

  const nextPosition = (exercises?.reduce((m, e) => Math.max(m, e.position ?? 0), 0) ?? 0) + 1;

  async function removeWorkout() {
    if (!confirm("Delete this workout and all its movements?")) return;
    try {
      await api.workout.delete(workout.id);
      onDeleted(workout.id);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not delete the workout.");
    }
  }

  async function removeExercise(id: string) {
    const prev = exercises;
    setExercises((xs) => xs?.filter((x) => x.id !== id) ?? null);
    try {
      await api.exercise.delete(id);
    } catch (err) {
      setExercises(prev ?? null); // roll back the optimistic removal
      setError(err instanceof ApiError ? err.message : "Could not delete the movement.");
    }
  }

  return (
    <div>
      <div className="back">
        <button className="link" onClick={onBack}>
          ← All workouts
        </button>
      </div>

      {error && <div className="error">{error}</div>}

      {editing ? (
        <WorkoutForm
          workout={workout}
          onSaved={(w) => {
            setWorkout(w);
            onChanged(w);
            setEditing(false);
          }}
          onCancel={() => setEditing(false)}
        />
      ) : (
        <div className="card">
          <div className="detail-head">
            <div>
              <h2>
                {typeEmoji(workout.type)} {workout.title || humanize(workout.type)}
              </h2>
              <div className="muted">{formatDate(workout.date)}</div>
            </div>
            <div className="badges">
              <span className="badge accent">{humanize(workout.type)}</span>
              {workout.completed ? <span className="badge ok">✓ done</span> : <span className="badge">planned</span>}
            </div>
          </div>
          <div className="stat-row">
            {workout.total_seconds != null && (
              <span>
                Total <b>{formatDuration(workout.total_seconds)}</b>
              </span>
            )}
            {workout.rpe != null && (
              <span>
                RPE <b>{workout.rpe}/10</b>
              </span>
            )}
            {workout.bodyweight_kg != null && (
              <span>
                Bodyweight <b>{workout.bodyweight_kg} kg</b>
              </span>
            )}
          </div>
          {workout.notes && <p className="muted" style={{ marginBottom: 0 }}>{workout.notes}</p>}
          <div className="actions">
            <button onClick={() => setEditing(true)}>Edit</button>
            <button className="danger" onClick={removeWorkout}>
              Delete
            </button>
          </div>
        </div>
      )}

      <div className="section-title">
        <h2>Movements</h2>
        {exercises && !adding && <button onClick={() => setAdding(true)}>+ Add movement</button>}
      </div>

      {exercises === null ? (
        <div className="spinner">Loading movements…</div>
      ) : exercises.length === 0 && !adding ? (
        <div className="empty">No movements logged yet.</div>
      ) : (
        exercises.length > 0 && (
          <div className="card" style={{ padding: 0, overflowX: "auto" }}>
            <table className="ex-table">
              <thead>
                <tr>
                  <th>#</th>
                  <th>Movement</th>
                  <th>Sets×Reps</th>
                  <th>Load</th>
                  <th>Dist</th>
                  <th>Time</th>
                  <th>Rest</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {exercises.map((ex, i) => (
                  <tr key={ex.id}>
                    <td className="muted">{ex.position ?? i + 1}</td>
                    <td>
                      {ex.personal_best && "🏆 "}
                      {ex.kind === "hyrox_station" && ex.station
                        ? humanize(ex.station)
                        : ex.name || humanize(ex.kind)}
                      <div className="muted" style={{ fontSize: 12 }}>{humanize(ex.kind)}</div>
                    </td>
                    <td>{ex.sets != null || ex.reps != null ? `${ex.sets ?? "—"}×${ex.reps ?? "—"}` : "—"}</td>
                    <td>{ex.weight_kg != null ? `${ex.weight_kg} kg` : "—"}</td>
                    <td>{ex.distance_m != null ? `${ex.distance_m} m` : "—"}</td>
                    <td>{formatDuration(ex.seconds)}</td>
                    <td>{formatDuration(ex.rest_seconds)}</td>
                    <td>
                      <button className="link danger" onClick={() => removeExercise(ex.id)} aria-label="Delete movement">
                        ✕
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      )}

      {adding && (
        <ExerciseForm
          workoutId={workout.id}
          position={nextPosition}
          onAdded={(ex) => {
            setExercises((xs) => [...(xs ?? []), ex]);
            setAdding(false);
          }}
          onCancel={() => setAdding(false)}
        />
      )}
    </div>
  );
}
