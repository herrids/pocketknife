import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useRegistry, useLastOpenedApp } from "../lib/useRegistry";
import type { RegistryEntry } from "../lib/api";
import { AppTile, NewAppTile } from "../components/AppTile";
import { NewAppSheet } from "../components/NewAppSheet";
import { ThemeToggle } from "../components/ThemeToggle";
// BottomNav (Home/Search/Recent/Menu) isn't wired to anything useful yet —
// commented out rather than deleted in case it gets a real menu later.
// import { BottomNav } from "../components/BottomNav";

const ACTIVE_STATES = new Set(["queued", "building", "activating"]);

function greeting(): string {
  const h = new Date().getHours();
  const day = new Date().toLocaleDateString("en-US", { weekday: "long" });
  let part = "morning";
  if (h >= 12 && h < 17) part = "afternoon";
  else if (h >= 17) part = "evening";
  return `${day} · ${part}`;
}

function buildPct(state: string): number {
  switch (state) {
    case "queued": return 5;
    case "building": return 45;
    case "activating": return 90;
    default: return 0;
  }
}

type FilterTab = "all" | "recent" | "pinned";

function filterApps(apps: RegistryEntry[], tab: FilterTab, lastAppId: string | null): RegistryEntry[] {
  if (tab === "recent" && lastAppId) {
    return apps.filter((a) => a.appId === lastAppId);
  }
  return apps;
}

export function Home() {
  const navigate = useNavigate();
  const { entries, loading } = useRegistry();
  const [lastAppId, recordOpen] = useLastOpenedApp();
  const [tab, setTab] = useState<FilterTab>("all");
  const [showSheet, setShowSheet] = useState(false);

  const buildingApps = entries.filter((e) => ACTIVE_STATES.has(e.buildState));
  const filteredApps = filterApps(entries, tab, lastAppId);

  function openApp(app: RegistryEntry) {
    recordOpen(app.appId);
    navigate(`/app/${app.appId}`);
  }

  return (
    <div className="min-h-dvh bg-canvas dark:bg-[#1B1813] pb-28 font-sans">
      {/* Header */}
      <div className="px-5 pt-12 pb-4">
        <div className="flex items-start justify-between">
          <div>
            {buildingApps.length > 0 ? (
              <p className="font-mono text-[11px] uppercase tracking-widest text-terracotta mb-0.5">
                Building {buildingApps.length} app{buildingApps.length > 1 ? "s" : ""}…
              </p>
            ) : (
              <p className="font-mono text-[11px] uppercase tracking-widest text-ink-muted dark:text-[#9c9282] mb-0.5">
                {greeting()}
              </p>
            )}
            <h1 className="text-[28px] font-bold text-ink dark:text-[#F3ECDD] leading-tight">
              Your pocketknife
            </h1>
          </div>
          <div className="flex items-center gap-2">
            <ThemeToggle />
            {/* Avatar */}
            <div className="w-11 h-11 rounded-[14px] bg-amber flex items-center justify-center text-xl"
              style={{ boxShadow: "0 3px 0 rgba(30,27,21,0.12)" }}>
              🙂
            </div>
          </div>
        </div>

        {/* Building progress card */}
        {buildingApps.length > 0 && (
          <div className="mt-4 rounded-2xl bg-ink dark:bg-[#0E0C09] p-4 text-surface dark:text-[#F3ECDD]">
            {buildingApps.map((app) => (
              <div key={app.appId} className="flex items-center gap-3">
                {/* Circular progress */}
                <div className="relative w-14 h-14 shrink-0">
                  <svg className="-rotate-90 w-14 h-14" viewBox="0 0 56 56">
                    <circle cx="28" cy="28" r="22" fill="none" stroke="rgba(255,255,255,0.15)" strokeWidth="4" />
                    <circle
                      cx="28" cy="28" r="22" fill="none"
                      stroke="#DD6440" strokeWidth="4"
                      strokeDasharray={`${2 * Math.PI * 22}`}
                      strokeDashoffset={`${2 * Math.PI * 22 * (1 - buildPct(app.buildState) / 100)}`}
                      strokeLinecap="round"
                    />
                  </svg>
                  <div className="absolute inset-0 flex items-center justify-center">
                    <span className="text-xs font-mono font-bold text-terracotta">{buildPct(app.buildState)}%</span>
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-mono text-[10px] uppercase tracking-widest text-white/50 mb-0.5">COMPILING</p>
                  <p className="font-semibold text-sm truncate">{app.displayName}</p>
                  <p className="text-xs text-white/40 mt-0.5">› {app.buildState}…</p>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Filter tabs */}
        <div className="flex gap-2 mt-4">
          {(["all", "recent", "pinned"] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-3.5 py-1.5 rounded-pill text-sm font-medium press-spring-sm transition-colors ${
                tab === t
                  ? "bg-ink dark:bg-[#F3ECDD] text-surface dark:text-ink"
                  : "border-[1.5px] border-ink/20 dark:border-white/20 text-ink-muted dark:text-[#9c9282]"
              }`}
            >
              {t === "all" ? "All apps" : t === "recent" ? "Recent" : "Pinned"}
            </button>
          ))}
        </div>
      </div>

      {/* App grid */}
      <div className="px-5">
        <div className="flex items-center justify-between mb-3">
          <p className="font-mono text-[10px] uppercase tracking-widest text-ink-muted dark:text-[#9c9282]">
            Your apps — {entries.length}
          </p>
          <button className="font-mono text-[10px] uppercase tracking-widest text-terracotta dark:text-[#E2724E]">
            EDIT
          </button>
        </div>

        {loading ? (
          <div className="py-16 text-center text-ink-muted text-sm">Loading…</div>
        ) : (
          <div className="grid grid-cols-4 gap-x-3.5 gap-y-4">
            {filteredApps.map((app) => (
              <AppTile key={app.appId} app={app} onClick={() => openApp(app)} />
            ))}
            <NewAppTile onClick={() => setShowSheet(true)} />
          </div>
        )}
      </div>

      {/* <BottomNav /> */}

      {showSheet && <NewAppSheet onClose={() => setShowSheet(false)} />}
    </div>
  );
}
