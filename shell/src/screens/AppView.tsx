import { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { api } from "../lib/api";
import type { RegistryEntry } from "../lib/api";
import { useLastOpenedApp } from "../lib/useRegistry";

export function AppView() {
  const { appId } = useParams<{ appId: string }>();
  const navigate = useNavigate();
  const [, recordOpen] = useLastOpenedApp();
  const [app, setApp] = useState<RegistryEntry | null>(null);
  const [changeInput, setChangeInput] = useState("");
  const [sendingChange, setSendingChange] = useState(false);

  useEffect(() => {
    if (!appId) return;
    recordOpen(appId);
    api.registry().then((entries) => {
      const found = entries.find((e) => e.appId === appId);
      if (found) setApp(found);
    });
  }, [appId, recordOpen]);

  async function handleChangeRequest() {
    if (!changeInput.trim() || !appId || sendingChange) return;
    const prompt = changeInput.trim();
    setChangeInput("");
    setSendingChange(true);
    try {
      const { sessionId } = await api.startPlan(prompt, appId);
      navigate(`/plan/${sessionId}`);
    } finally {
      setSendingChange(false);
    }
  }

  return (
    <div className="min-h-dvh bg-surface dark:bg-[#1B1813] flex flex-col font-sans">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 pt-12 pb-3 bg-surface dark:bg-[#1B1813] border-b border-ink/8 dark:border-white/8">
        <button
          onClick={() => navigate("/home")}
          className="text-ink-muted dark:text-[#9c9282] text-xl press-spring-sm"
        >
          ‹
        </button>
        {app && (
          <>
            <div
              className="w-8 h-8 rounded-[10px] flex items-center justify-center text-base"
              style={{ backgroundColor: app.color }}
            >
              {app.emoji}
            </div>
            <span className="flex-1 font-semibold text-ink dark:text-[#F3ECDD] truncate">
              {app.displayName}
            </span>
          </>
        )}
        <button className="text-ink-muted dark:text-[#9c9282] text-xl press-spring-sm">
          ⋯
        </button>
      </div>

      {/* App iframe */}
      <div className="flex-1 relative">
        {appId && (
          <iframe
            src={`/ui/${appId}/`}
            className="absolute inset-0 w-full h-full border-none"
            sandbox="allow-scripts allow-same-origin allow-forms"
            title={app?.displayName ?? appId}
          />
        )}
      </div>

      {/* Inline change request bar */}
      <div className="px-4 py-3 bg-surface dark:bg-[#1B1813] border-t border-ink/8 dark:border-white/8 safe-area-inset-bottom">
        <div className="flex gap-2 items-center">
          <span className="text-lg">✦</span>
          <input
            type="text"
            value={changeInput}
            onChange={(e) => setChangeInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleChangeRequest()}
            placeholder="Ask pocketknife to change this app…"
            className="flex-1 bg-transparent text-sm text-ink dark:text-[#F3ECDD] placeholder:text-ink-faint outline-none"
          />
          {changeInput.trim() && (
            <button
              onClick={handleChangeRequest}
              disabled={sendingChange}
              className="w-8 h-8 rounded-full bg-terracotta text-white flex items-center justify-center text-sm press-spring-sm disabled:opacity-40"
            >
              ↑
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
