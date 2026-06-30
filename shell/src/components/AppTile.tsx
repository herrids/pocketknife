import type { RegistryEntry } from "../lib/api";

interface AppTileProps {
  app: RegistryEntry;
  onClick: () => void;
}

const ACTIVE_STATES = new Set(["queued", "building", "activating"]);

function buildProgress(state: string): number {
  switch (state) {
    case "queued": return 5;
    case "building": return 45;
    case "activating": return 90;
    case "ready": return 100;
    default: return 0;
  }
}

export function AppTile({ app, onClick }: AppTileProps) {
  const isBuilding = ACTIVE_STATES.has(app.buildState);
  const isFailed = app.buildState === "failed";
  const pct = buildProgress(app.buildState);

  return (
    <button
      onClick={onClick}
      className="flex flex-col items-center gap-1.5 press-spring group"
    >
      {/* Icon square */}
      <div
        className="relative w-[70px] h-[70px] rounded-squircle flex items-center justify-center text-3xl"
        style={{
          backgroundColor: app.color,
          boxShadow: "0 3px 0 rgba(30,27,21,0.10)",
          opacity: isBuilding ? 0.7 : 1,
        }}
      >
        <span>{app.emoji}</span>

        {/* Progress ring overlay */}
        {isBuilding && (
          <div className="absolute inset-0 rounded-squircle flex items-center justify-center">
            <svg className="w-14 h-14 -rotate-90 absolute" viewBox="0 0 56 56">
              <circle cx="28" cy="28" r="24" fill="none" stroke="rgba(255,255,255,0.3)" strokeWidth="3" />
              <circle
                cx="28" cy="28" r="24" fill="none"
                stroke="white" strokeWidth="3"
                strokeDasharray={`${2 * Math.PI * 24}`}
                strokeDashoffset={`${2 * Math.PI * 24 * (1 - pct / 100)}`}
                strokeLinecap="round"
              />
            </svg>
          </div>
        )}

        {/* Failed badge */}
        {isFailed && (
          <div className="absolute -top-1 -right-1 w-5 h-5 rounded-full bg-red-500 text-white text-xs flex items-center justify-center font-bold">
            !
          </div>
        )}
      </div>

      {/* Label */}
      <span
        className={`text-[11px] font-medium text-center leading-tight max-w-[70px] truncate text-ink dark:text-[#F3ECDD] ${
          isBuilding ? "animate-pocket-pulse" : ""
        }`}
      >
        {isBuilding ? "Building…" : app.displayName}
      </span>
    </button>
  );
}

export function NewAppTile({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex flex-col items-center gap-1.5 press-spring"
    >
      <div
        className="w-[70px] h-[70px] rounded-squircle flex items-center justify-center text-2xl border-2 border-dashed border-terracotta/60"
        style={{ backgroundColor: "transparent" }}
      >
        <span className="text-terracotta">+</span>
      </div>
      <span className="text-[11px] font-medium text-ink-muted dark:text-[#9c9282]">New</span>
    </button>
  );
}
