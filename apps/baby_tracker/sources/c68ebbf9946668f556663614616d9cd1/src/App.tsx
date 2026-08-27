import { useState } from "react";
import { Heart, Zap, Star, Eye, BookOpen, Sun, Moon } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { useTheme } from "@/lib/use-theme";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import BabyView from "@/components/baby/BabyView";
import SprungView from "@/components/sprung/SprungView";
import FaehigkeitView from "@/components/faehigkeit/FaehigkeitView";
import BeobachtungView from "@/components/beobachtung/BeobachtungView";
import TagebuchView from "@/components/tagebuch/TagebuchView";

type Tab = "baby" | "sprung" | "faehigkeit" | "beobachtung" | "tagebuch";

interface TabItem {
  id: Tab;
  label: string;
  Icon: LucideIcon;
}

const TABS: TabItem[] = [
  { id: "baby", label: "Baby", Icon: Heart },
  { id: "sprung", label: "Sprünge", Icon: Zap },
  { id: "faehigkeit", label: "Fähig.", Icon: Star },
  { id: "beobachtung", label: "Beob.", Icon: Eye },
  { id: "tagebuch", label: "Tagebuch", Icon: BookOpen },
];

export default function App() {
  const [activeTab, setActiveTab] = useState<Tab>("baby");
  const { theme, toggle } = useTheme();

  return (
    <div className="min-h-dvh bg-background text-foreground flex flex-col">
      <header className="sticky top-0 z-40 border-b bg-background">
        <div className="mx-auto max-w-2xl px-4 h-14 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-xl" aria-hidden="true">👶</span>
            <h1 className="text-base font-semibold tracking-tight">Babyentwicklung</h1>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={toggle}
            aria-label={theme === "dark" ? "Helles Design aktivieren" : "Dunkles Design aktivieren"}
          >
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </Button>
        </div>
      </header>

      <main className="flex-1 mx-auto w-full max-w-2xl px-4 py-5 pb-24 overflow-x-hidden">
        {activeTab === "baby" && <BabyView />}
        {activeTab === "sprung" && <SprungView />}
        {activeTab === "faehigkeit" && <FaehigkeitView />}
        {activeTab === "beobachtung" && <BeobachtungView />}
        {activeTab === "tagebuch" && <TagebuchView />}
      </main>

      <nav className="fixed bottom-0 left-0 right-0 z-40 border-t bg-background">
        <div className="mx-auto max-w-2xl flex pb-2">
          {TABS.map(({ id, label, Icon }) => (
            <button
              key={id}
              onClick={() => setActiveTab(id)}
              className={cn(
                "flex-1 flex flex-col items-center gap-0.5 pt-2 pb-1 text-[10px] font-medium leading-tight transition-colors",
                activeTab === id
                  ? "text-primary"
                  : "text-muted-foreground hover:text-foreground"
              )}
              aria-label={label}
              aria-current={activeTab === id ? "page" : undefined}
            >
              <Icon className="h-5 w-5" />
              <span>{label}</span>
            </button>
          ))}
        </div>
      </nav>

      <Toaster richColors closeButton />
    </div>
  );
}
