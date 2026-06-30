import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../lib/api";

const SUGGESTIONS = [
  { emoji: "🪴", label: "Plant waterer" },
  { emoji: "🙏", label: "Gratitude log" },
  { emoji: "🧾", label: "Split the bill" },
  { emoji: "📚", label: "Reading tracker" },
  { emoji: "💧", label: "Water tracker" },
  { emoji: "🏋️", label: "Workout log" },
  { emoji: "💸", label: "Budget tracker" },
  { emoji: "✍️", label: "Journal" },
  { emoji: "🍳", label: "Recipe box" },
];

function shuffled<T>(arr: T[]): T[] {
  const a = [...arr];
  for (let i = a.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

interface NewAppSheetProps {
  onClose: () => void;
}

export function NewAppSheet({ onClose }: NewAppSheetProps) {
  const navigate = useNavigate();
  const [tab, setTab] = useState<"describe" | "paste">("describe");
  const [text, setText] = useState("");
  const [loading, setLoading] = useState(false);
  const [suggestions] = useState(() => shuffled(SUGGESTIONS).slice(0, 3));

  async function handleCreate() {
    if (!text.trim()) return;
    setLoading(true);
    try {
      const { sessionId } = await api.startPlan(text.trim());
      navigate(`/plan/${sessionId}`);
      onClose();
    } catch {
      setLoading(false);
    }
  }

  return (
    <>
      {/* Scrim */}
      <div
        className="fixed inset-0 bg-ink/40 z-40"
        onClick={onClose}
      />

      {/* Sheet */}
      <div className="fixed bottom-0 left-0 right-0 z-50 bg-surface dark:bg-[#1B1813] rounded-t-[30px] px-5 pt-3 pb-10 max-h-[85dvh] flex flex-col animate-spring-in">
        {/* Drag handle */}
        <div className="w-10 h-1.5 rounded-pill bg-ink/20 dark:bg-white/20 mx-auto mb-4" />

        {/* Header */}
        <div className="flex items-center justify-between mb-5">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-widest text-ink-muted dark:text-[#9c9282]">
              Describe a tool
            </p>
            <h2 className="text-[22px] font-bold text-ink dark:text-[#F3ECDD]">New app</h2>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-ink/10 dark:bg-white/10 flex items-center justify-center text-ink-muted press-spring-sm"
          >
            ✕
          </button>
        </div>

        {/* Mode tabs */}
        <div className="flex gap-2 mb-4">
          {(["describe", "paste"] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`px-4 py-1.5 rounded-pill text-sm font-medium press-spring-sm transition-colors ${
                tab === t
                  ? "bg-ink dark:bg-[#F3ECDD] text-surface dark:text-ink"
                  : "border-[1.5px] border-ink/20 dark:border-white/20 text-ink-muted dark:text-[#9c9282]"
              }`}
            >
              {t === "describe" ? "Describe it" : "Paste code"}
            </button>
          ))}
        </div>

        {/* Text area */}
        <div className="flex-1 relative min-h-[120px]">
          <textarea
            className="w-full h-full min-h-[120px] resize-none bg-card dark:bg-[#28231C] rounded-xl p-4 text-sm text-ink dark:text-[#F3ECDD] placeholder:text-ink-faint outline-none focus:ring-2 focus:ring-terracotta/30"
            placeholder={
              tab === "describe"
                ? "A tracker for the books I'm reading — let me add a title and author, log pages, and mark when I finish."
                : "Paste your frontend code here…"
            }
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <div className="absolute bottom-3 right-3 text-[10px] font-mono text-ink-faint dark:text-[#6f6a60]">
            {text.length}
          </div>
        </div>

        {/* Suggestions */}
        <div className="mt-4">
          <p className="text-[10px] font-mono uppercase tracking-widest text-ink-muted dark:text-[#9c9282] mb-2">
            Or start from an idea
          </p>
          <div className="flex gap-2">
            {suggestions.map((s) => (
              <button
                key={s.label}
                onClick={() => setText(s.label)}
                className="flex-1 py-2.5 px-2 rounded-xl bg-card dark:bg-[#28231C] border border-ink/8 dark:border-white/8 text-center press-spring-sm"
              >
                <div className="text-xl mb-0.5">{s.emoji}</div>
                <div className="text-[10px] text-ink-muted dark:text-[#9c9282] leading-tight">{s.label}</div>
              </button>
            ))}
          </div>
        </div>

        {/* CTA */}
        <button
          onClick={handleCreate}
          disabled={!text.trim() || loading}
          className="mt-5 w-full py-3.5 rounded-xl bg-ink dark:bg-[#F3ECDD] text-surface dark:text-ink font-semibold press-spring-sm disabled:opacity-40"
        >
          {loading ? "Starting…" : "Create app →"}
        </button>
      </div>
    </>
  );
}
