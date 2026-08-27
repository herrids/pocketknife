import { useState } from "react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import { Plus, Swords, Trophy, Clock } from "lucide-react";
import { ApiError } from "@/client";
import type { Game } from "@/client";
import { useGames } from "@/hooks/useGames";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

type GameCreateForm = {
  player1_name: string;
  player2_name: string;
  question_count: string;
};

function emptyForm(): GameCreateForm {
  return { player1_name: "", player2_name: "", question_count: "10" };
}

function validate(f: GameCreateForm): Partial<GameCreateForm> {
  const e: Partial<GameCreateForm> = {};
  if (!f.player1_name.trim()) e.player1_name = "Pflichtfeld";
  if (!f.player2_name.trim()) e.player2_name = "Pflichtfeld";
  const count = parseInt(f.question_count, 10);
  if (isNaN(count) || count < 1) e.question_count = "Mindestens 1 Runde";
  return e;
}

interface GameListProps {
  onSelectGame: (id: string) => void;
}

export default function GameList({ onSelectGame }: GameListProps) {
  const { games, loading, create } = useGames();
  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState<GameCreateForm>(emptyForm());
  const [errors, setErrors] = useState<Partial<GameCreateForm>>({});
  const [saving, setSaving] = useState(false);

  function setField(field: keyof GameCreateForm, value: string) {
    setForm((f) => ({ ...f, [field]: value }));
    setErrors((e) => ({ ...e, [field]: undefined }));
  }

  async function handleCreate() {
    const errs = validate(form);
    if (Object.keys(errs).length > 0) {
      setErrors(errs);
      return;
    }
    setSaving(true);
    try {
      const game = await create({
        player1_name: form.player1_name.trim(),
        player2_name: form.player2_name.trim(),
        question_count: parseInt(form.question_count, 10),
      });
      setCreateOpen(false);
      setForm(emptyForm());
      setErrors({});
      onSelectGame(game.id);
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Erstellen");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Spiele</h2>
          {!loading && (
            <p className="text-sm text-muted-foreground">
              {games.length} {games.length === 1 ? "Spiel" : "Spiele"}
            </p>
          )}
        </div>
        <Button onClick={() => setCreateOpen(true)} size="sm" className="gap-1.5">
          <Plus className="h-4 w-4" />
          Neues Spiel
        </Button>
      </div>

      {loading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full rounded-xl" />
          ))}
        </div>
      ) : games.length === 0 ? (
        <div className="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed py-16 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-muted text-3xl">
            ⚽
          </div>
          <div>
            <p className="font-medium">Noch kein Spiel</p>
            <p className="mt-1 text-sm text-muted-foreground">
              Starte das erste Schätzduell!
            </p>
          </div>
          <Button
            onClick={() => setCreateOpen(true)}
            variant="outline"
            size="sm"
            className="gap-1.5"
          >
            <Plus className="h-4 w-4" />
            Erstes Spiel starten
          </Button>
        </div>
      ) : (
        <motion.div
          className="space-y-3"
          initial="hidden"
          animate="visible"
          variants={{ visible: { transition: { staggerChildren: 0.05 } } }}
        >
          <AnimatePresence mode="popLayout">
            {games.map((game) => (
              <motion.div
                key={game.id}
                variants={{
                  hidden: { opacity: 0, y: 8 },
                  visible: { opacity: 1, y: 0, transition: { duration: 0.2 } },
                }}
                exit={{ opacity: 0, y: -8, transition: { duration: 0.15 } }}
                layout
              >
                <GameCard game={game} onClick={() => onSelectGame(game.id)} />
              </motion.div>
            ))}
          </AnimatePresence>
        </motion.div>
      )}

      <Dialog open={createOpen} onOpenChange={(o) => !o && setCreateOpen(false)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Swords className="h-5 w-5" />
              Neues Duell
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="g-p1">Spieler 1 *</Label>
              <Input
                id="g-p1"
                placeholder="Name des ersten Spielers"
                value={form.player1_name}
                onChange={(e) => setField("player1_name", e.target.value)}
                className={cn(errors.player1_name && "border-destructive")}
                autoFocus
              />
              {errors.player1_name && (
                <p className="text-xs text-destructive">{errors.player1_name}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="g-p2">Spieler 2 *</Label>
              <Input
                id="g-p2"
                placeholder="Name des zweiten Spielers"
                value={form.player2_name}
                onChange={(e) => setField("player2_name", e.target.value)}
                className={cn(errors.player2_name && "border-destructive")}
              />
              {errors.player2_name && (
                <p className="text-xs text-destructive">{errors.player2_name}</p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="g-count">Anzahl Runden</Label>
              <Input
                id="g-count"
                type="number"
                min={1}
                value={form.question_count}
                onChange={(e) => setField("question_count", e.target.value)}
                className={cn(errors.question_count && "border-destructive")}
              />
              {errors.question_count && (
                <p className="text-xs text-destructive">{errors.question_count}</p>
              )}
            </div>
          </div>
          <DialogFooter className="gap-2 sm:gap-0">
            <DialogClose asChild>
              <Button variant="outline" disabled={saving}>
                Abbrechen
              </Button>
            </DialogClose>
            <Button onClick={handleCreate} disabled={saving}>
              {saving ? "Wird gestartet…" : "Duell starten ⚽"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function GameCard({ game, onClick }: { game: Game; onClick: () => void }) {
  const isActive = game.status === "active";
  const totalRounds = game.question_count;
  const completedRounds = Math.max(0, game.current_round - 1);

  return (
    <Card
      className={cn(
        "cursor-pointer overflow-hidden transition-all hover:shadow-md active:scale-[0.99]",
        isActive && "ring-1 ring-primary/30",
      )}
      onClick={onClick}
    >
      <CardContent className="p-4">
        <div className="flex items-center gap-3">
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-2">
              <Badge
                className={cn(
                  "border-0 text-xs font-medium",
                  isActive
                    ? "bg-primary/15 text-primary"
                    : "bg-muted text-muted-foreground",
                )}
              >
                {isActive ? (
                  <span className="flex items-center gap-1">
                    <Clock className="h-3 w-3" />
                    Aktiv
                  </span>
                ) : (
                  <span className="flex items-center gap-1">
                    <Trophy className="h-3 w-3" />
                    Beendet
                  </span>
                )}
              </Badge>
              {isActive && (
                <span className="text-xs text-muted-foreground">
                  Runde {completedRounds + 1} / {totalRounds}
                </span>
              )}
            </div>
            <div className="flex items-center gap-2">
              <span className="font-semibold truncate">{game.player1_name}</span>
              <span className="text-2xl font-bold tabular-nums text-primary">
                {game.player1_score}
              </span>
              <span className="text-muted-foreground font-medium px-0.5">vs</span>
              <span className="text-2xl font-bold tabular-nums text-primary">
                {game.player2_score}
              </span>
              <span className="font-semibold truncate">{game.player2_name}</span>
            </div>
            {!isActive && (
              <p className="mt-1 text-xs text-muted-foreground">
                {totalRounds} Runden gespielt
              </p>
            )}
          </div>
          <div className="text-muted-foreground/40 shrink-0">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
