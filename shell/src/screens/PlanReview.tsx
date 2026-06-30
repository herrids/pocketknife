import { useState, useEffect, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { api } from "../lib/api";

interface ChatMessage {
  role: "user" | "assistant";
  text: string;
}

interface CheckItem {
  text: string;
  done: boolean;
}

interface BridgeEvent {
  type: string;
  role?: string;
  text?: string;
  checklist?: CheckItem[];
  manifestVersion?: number;
  appId?: string;
  reason?: string;
}

export function PlanReview() {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [checklist, setChecklist] = useState<CheckItem[]>([]);
  const [isReady, setIsReady] = useState(false);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [approving, setApproving] = useState(false);
  const [disconnected, setDisconnected] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!sessionId) return;
    let lastId = "";

    function connect() {
      const es = new EventSource(api.eventsUrl(sessionId!));
      esRef.current = es;
      setDisconnected(false);

      es.onmessage = (e) => {
        try {
          const ev: BridgeEvent = JSON.parse(e.data);
          lastId = e.lastEventId;

          if (ev.type === "turn" && ev.role === "assistant" && ev.text) {
            setMessages((prev) => [...prev, { role: "assistant", text: ev.text! }]);
          }
          if (ev.type === "plan" && ev.checklist) {
            setChecklist(ev.checklist);
          }
          if (ev.type === "ready") {
            setIsReady(true);
          }
          if (ev.type === "done" || ev.type === "error") {
            es.close();
          }
        } catch { /* ignore */ }
      };

      es.onerror = () => {
        es.close();
        setDisconnected(true);
        // Exponential backoff reconnect.
        setTimeout(() => connect(), 2000);
      };
    }

    connect();
    return () => esRef.current?.close();
  }, [sessionId]);

  // Scroll to bottom on new messages.
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  async function handleSend() {
    if (!input.trim() || !sessionId || sending) return;
    const text = input.trim();
    setInput("");
    setSending(true);
    setMessages((prev) => [...prev, { role: "user", text }]);
    try {
      await api.sendMessage(sessionId, text);
    } finally {
      setSending(false);
    }
  }

  async function handleApprove() {
    if (!sessionId || approving) return;
    setApproving(true);
    try {
      await api.approvePlan(sessionId);
      navigate("/home");
    } finally {
      setApproving(false);
    }
  }

  return (
    <div className="min-h-dvh bg-surface dark:bg-[#1B1813] flex flex-col font-sans">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 pt-12 pb-4 border-b border-ink/8 dark:border-white/8">
        <button onClick={() => navigate("/home")} className="text-ink-muted dark:text-[#9c9282] text-xl press-spring-sm">
          ‹
        </button>
        <div className="flex-1">
          <p className="font-mono text-[10px] uppercase tracking-widest text-ink-muted dark:text-[#9c9282]">
            STEP 2 OF 2 · REVIEW
          </p>
        </div>
      </div>

      {/* Disconnected banner */}
      {disconnected && (
        <div className="bg-amber/20 text-amber px-4 py-2 text-xs text-center font-mono">
          Reconnecting…
        </div>
      )}

      {/* Checklist */}
      {checklist.length > 0 && (
        <div className="mx-4 mt-4 rounded-2xl bg-card dark:bg-[#28231C] p-4 border border-ink/8 dark:border-white/8">
          <p className="text-[11px] font-semibold text-ink dark:text-[#F3ECDD] mb-2">What I'll build</p>
          <ul className="space-y-1.5">
            {checklist.map((item, i) => (
              <li key={i} className="flex items-start gap-2 text-sm text-ink-muted dark:text-[#9c9282]">
                <span className="w-4 h-4 rounded-full bg-terracotta text-white text-[9px] flex items-center justify-center shrink-0 mt-0.5">✓</span>
                {item.text}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Chat transcript */}
      <div className="flex-1 overflow-y-auto px-4 py-4 space-y-3">
        {messages.length === 0 && (
          <p className="text-ink-faint text-sm text-center py-8">Planning your app…</p>
        )}
        {messages.map((msg, i) => (
          <div key={i} className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}>
            <div
              className={`max-w-[80%] rounded-2xl px-4 py-2.5 text-sm leading-relaxed ${
                msg.role === "user"
                  ? "bg-terracotta text-white rounded-br-sm"
                  : "bg-card dark:bg-[#28231C] text-ink dark:text-[#F3ECDD] rounded-bl-sm border border-ink/8 dark:border-white/8"
              }`}
            >
              {msg.text}
            </div>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Actions */}
      <div className="px-4 pb-8 space-y-3">
        {isReady && (
          <button
            onClick={handleApprove}
            disabled={approving}
            className="w-full py-3.5 rounded-xl bg-ink dark:bg-[#F3ECDD] text-surface dark:text-ink font-semibold press-spring-sm disabled:opacity-40"
          >
            {approving ? "Building…" : "Looks good — build it →"}
          </button>
        )}
        <div className="flex gap-2">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSend()}
            placeholder="Request a change…"
            className="flex-1 px-4 py-3 rounded-xl bg-card dark:bg-[#28231C] border border-ink/10 dark:border-white/10 text-sm text-ink dark:text-[#F3ECDD] placeholder:text-ink-faint outline-none focus:ring-2 focus:ring-terracotta/30"
          />
          <button
            onClick={handleSend}
            disabled={!input.trim() || sending}
            className="w-11 h-11 rounded-xl bg-terracotta text-white flex items-center justify-center press-spring-sm disabled:opacity-40"
          >
            ↑
          </button>
        </div>
      </div>
    </div>
  );
}
