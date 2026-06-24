"use client";

import { useEffect, useState } from "react";
import { api, ApiError, type Workout } from "@/lib/api";
import { formatDate, formatDuration, humanize, typeEmoji } from "@/lib/format";
import WorkoutForm from "./components/WorkoutForm";
import WorkoutDetail from "./components/WorkoutDetail";

// This is a single-page client app. Pocketknife serves the static export
// behind an SPA entry-file fallback, so we drive all navigation from in-memory
// view state rather than separate routed pages — a hard refresh on any path
// always lands back here and rebuilds from the API.
type View = { kind: "list" } | { kind: "new" } | { kind: "detail"; id: string };

export default function Page() {
  const [workouts, setWorkouts] = useState<Workout[] | null>(null);
  const [view, setView] = useState<View>({ kind: "list" });
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void load();
  }, []);

  async function load() {
    setError(null);
    try {
      const res = await api.workout.list({ sort: ["-date"], limit: 200 });
      setWorkouts(res.data);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not reach the server.");
      setWorkouts([]);
    }
  }

  const selected = view.kind === "detail" ? workouts?.find((w) => w.id === view.id) : undefined;

  // If a detail id no longer resolves (e.g. after a delete elsewhere), fall back.
  if (view.kind === "detail" && workouts && !selected) {
    return <Shell>{renderList()}</Shell>;
  }

  function renderList() {
    return (
      <>
        <div className="section-title">
          <h2>{workouts ? `${workouts.length} session${workouts.length === 1 ? "" : "s"}` : "Sessions"}</h2>
          <button className="primary" onClick={() => setView({ kind: "new" })}>
            + New workout
          </button>
        </div>

        {error && <div className="error">{error}</div>}

        {workouts === null ? (
          <div className="spinner">Loading…</div>
        ) : workouts.length === 0 ? (
          <div className="empty">
            No workouts yet.
            <br />
            Log your first session to get started.
          </div>
        ) : (
          <div className="list">
            {workouts.map((w) => (
              <div key={w.id} className="row" onClick={() => setView({ kind: "detail", id: w.id })}>
                <div className="lead">{typeEmoji(w.type)}</div>
                <div className="grow">
                  <div className="title">{w.title || humanize(w.type)}</div>
                  <div className="meta">{formatDate(w.date)}</div>
                </div>
                <div className="badges">
                  {w.total_seconds != null && <span className="badge">{formatDuration(w.total_seconds)}</span>}
                  {w.rpe != null && <span className="badge">RPE {w.rpe}</span>}
                  {w.completed ? <span className="badge ok">✓</span> : <span className="badge">planned</span>}
                </div>
              </div>
            ))}
          </div>
        )}
      </>
    );
  }

  function renderBody() {
    if (view.kind === "new") {
      return (
        <>
          <div className="back">
            <button className="link" onClick={() => setView({ kind: "list" })}>
              ← All workouts
            </button>
          </div>
          <WorkoutForm
            onSaved={(w) => {
              setWorkouts((xs) => [w, ...(xs ?? [])]);
              setView({ kind: "detail", id: w.id });
            }}
            onCancel={() => setView({ kind: "list" })}
          />
        </>
      );
    }
    if (view.kind === "detail" && selected) {
      return (
        <WorkoutDetail
          workout={selected}
          onBack={() => setView({ kind: "list" })}
          onChanged={(w) => setWorkouts((xs) => xs?.map((x) => (x.id === w.id ? w : x)) ?? null)}
          onDeleted={(id) => {
            setWorkouts((xs) => xs?.filter((x) => x.id !== id) ?? null);
            setView({ kind: "list" });
          }}
        />
      );
    }
    return renderList();
  }

  return <Shell>{renderBody()}</Shell>;
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <main className="app">
      <header className="topbar">
        <h1>🏋️ Training</h1>
        <span className="sub">HYROX &amp; strength log</span>
      </header>
      {children}
    </main>
  );
}
