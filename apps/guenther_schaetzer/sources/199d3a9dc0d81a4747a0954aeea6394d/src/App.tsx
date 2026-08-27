import { useState } from "react";
import { Sun, Moon } from "lucide-react";
import { Toaster } from "@/components/ui/sonner";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { useTheme } from "@/lib/use-theme";
import GameList from "@/components/games/GameList";
import GameDetail from "@/components/games/GameDetail";
import QuestionList from "@/components/questions/QuestionList";

type Tab = "spiele" | "fragen";

export default function App() {
  const { theme, toggle } = useTheme();
  const [tab, setTab] = useState<Tab>("spiele");
  const [selectedGameId, setSelectedGameId] = useState<string | null>(null);

  function handleSelectGame(id: string) {
    setSelectedGameId(id);
  }

  function handleBackToGames() {
    setSelectedGameId(null);
  }

  function handleTabChange(value: string) {
    setTab(value as Tab);
    setSelectedGameId(null);
  }

  const showingGameDetail = tab === "spiele" && selectedGameId !== null;

  return (
    <div className="min-h-dvh bg-background text-foreground">
      {/* Header */}
      <header className="sticky top-0 z-40 border-b bg-background/90 backdrop-blur-sm">
        <div className="mx-auto flex h-14 max-w-2xl items-center justify-between px-4">
          <button
            type="button"
            className="flex items-center gap-2 focus:outline-none"
            onClick={() => { setTab("spiele"); setSelectedGameId(null); }}
          >
            <span className="text-2xl leading-none">⚽</span>
            <span className="font-bold tracking-tight">Günther Schätzer</span>
          </button>
          <Button
            variant="ghost"
            size="icon"
            onClick={toggle}
            aria-label={theme === "dark" ? "Helles Design" : "Dunkles Design"}
            className="h-9 w-9"
          >
            {theme === "dark" ? (
              <Sun className="h-4 w-4" />
            ) : (
              <Moon className="h-4 w-4" />
            )}
          </Button>
        </div>
      </header>

      {/* Tab navigation — hidden while inside a game detail */}
      {!showingGameDetail && (
        <div className="border-b bg-background">
          <div className="mx-auto max-w-2xl px-4 py-2">
            <Tabs value={tab} onValueChange={handleTabChange}>
              <TabsList className="w-full">
                <TabsTrigger value="spiele" className="flex-1">
                  🏆 Spiele
                </TabsTrigger>
                <TabsTrigger value="fragen" className="flex-1">
                  📝 Fragen
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
        </div>
      )}

      {/* Main content */}
      <main className="mx-auto max-w-2xl px-4 py-6 pb-16">
        {tab === "spiele" && !selectedGameId && (
          <GameList onSelectGame={handleSelectGame} />
        )}
        {tab === "spiele" && selectedGameId && (
          <GameDetail gameId={selectedGameId} onBack={handleBackToGames} />
        )}
        {tab === "fragen" && <QuestionList />}
      </main>

      <Toaster richColors closeButton position="top-center" />
    </div>
  );
}
