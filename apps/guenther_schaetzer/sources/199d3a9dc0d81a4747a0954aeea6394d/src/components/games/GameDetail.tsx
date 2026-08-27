import { useState } from "react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "framer-motion";
import {
  ArrowLeft,
  ChevronRight,
  Trophy,
  Target,
  Flag,
  Search,
} from "lucide-react";
import { ApiError } from "@/client";
import type { Question, RoundWinner, QuestionCategory, QuestionDifficulty } from "@/client";
import { useGame } from "@/hooks/useGame";
import { useRounds } from "@/hooks/useRounds";
import { useQuestions } from "@/hooks/useQuestions";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import {
  CATEGORY_LABELS,
  CATEGORY_COLORS,
  CATEGORY_EMOJIS,
  DIFFICULTY_LABELS,
  DIFFICULTY_COLORS,
} from "@/lib/labels";

interface GameDetailProps {
  gameId: string;
  onBack: () => void;
}

export default function GameDetail({ gameId, onBack }: GameDetailProps) {
  const { game, loading: gameLoading, update: updateGame } = useGame(gameId);
  const { rounds, loading: roundsLoading, create: createRound, update: updateRound } = useRounds(gameId);
  const { questions, loading: questionsLoading } = useQuestions();
  const [pickOpen, setPickOpen] = useState(false);
  const [catFilter, setCatFilter] = useState<string>("all");
  const [diffFilter, setDiffFilter] = useState<string>("all");
  const [selectedQuestionId, setSelectedQuestionId] = useState<string | null>(null);
  const [startingRound, setStartingRound] = useState(false);
  const [p1GuessInput, setP1GuessInput] = useState("");
  const [p2GuessInput, setP2GuessInput] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (gameLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-48 rounded-lg" />
        <Skeleton className="h-32 w-full rounded-xl" />
        <Skeleton className="h-52 w-full rounded-xl" />
      </div>
    );
  }

  if (!game) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 py-16 text-center">
        <p className="text-muted-foreground">Spiel nicht gefunden</p>
        <Button variant="outline" onClick={onBack}>
          Zurück
        </Button>
      </div>
    );
  }

  const questionMap = new Map<string, Question>(questions.map((q): [string, Question] => [q.id, q]));

  const revealedRounds = rounds.filter((r) => r.status === "revealed");
  const activeRound = rounds.find((r) => r.status !== "revealed") ?? null;
  const isGameFinished = game.status === "finished";
  const hasMaxRounds = game.current_round > game.question_count;

  // --- Filtered questions for picker ---
  const filteredQuestions = questions.filter((q) => {
    const usedIds = new Set(rounds.map((r) => r.question));
    if (usedIds.has(q.id)) return false;
    if (catFilter !== "all" && q.category !== catFilter) return false;
    if (diffFilter !== "all" && q.difficulty !== diffFilter) return false;
    return true;
  });

  async function handleStartRound() {
    if (!selectedQuestionId || !game) return;
    setStartingRound(true);
    try {
      await createRound({
        game: game.id,
        question: selectedQuestionId,
        round_number: game.current_round,
        status: "waiting_p1",
      });
      setPickOpen(false);
      setSelectedQuestionId(null);
      setCatFilter("all");
      setDiffFilter("all");
      setP1GuessInput("");
      setP2GuessInput("");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Starten der Runde");
    } finally {
      setStartingRound(false);
    }
  }

  async function handleP1Submit() {
    if (!activeRound || !game) return;
    const guess = parseInt(p1GuessInput, 10);
    if (isNaN(guess)) {
      toast.error("Bitte eine gültige Zahl eingeben");
      return;
    }
    setSubmitting(true);
    try {
      await updateRound(activeRound.id, { player1_guess: guess, status: "waiting_p2" });
      setP1GuessInput("");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Speichern");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleReveal() {
    if (!activeRound || !game) return;
    const guess = parseInt(p2GuessInput, 10);
    if (isNaN(guess)) {
      toast.error("Bitte eine gültige Zahl eingeben");
      return;
    }
    const question = questionMap.get(activeRound.question);
    if (!question) {
      toast.error("Frage nicht gefunden");
      return;
    }
    setSubmitting(true);
    try {
      const p1g = activeRound.player1_guess ?? 0;
      const dist1 = Math.abs(p1g - question.correct_answer);
      const dist2 = Math.abs(guess - question.correct_answer);
      let winner: RoundWinner;
      if (dist1 < dist2) winner = "player1";
      else if (dist2 < dist1) winner = "player2";
      else winner = "tie";

      await updateRound(activeRound.id, {
        player2_guess: guess,
        status: "revealed",
        winner,
      });

      const newP1Score = game.player1_score + (winner === "player1" ? 1 : 0);
      const newP2Score = game.player2_score + (winner === "player2" ? 1 : 0);
      const nextRound = game.current_round + 1;
      const newStatus = nextRound > game.question_count ? "finished" : "active";

      await updateGame({
        player1_score: newP1Score,
        player2_score: newP2Score,
        current_round: nextRound,
        status: newStatus,
      });

      setP2GuessInput("");
      if (winner === "player1") toast.success(`${game.player1_name} gewinnt die Runde! 🎉`);
      else if (winner === "player2") toast.success(`${game.player2_name} gewinnt die Runde! 🎉`);
      else toast.success("Unentschieden – beide gleich nah! 🤝");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler beim Auflösen");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleEndGame() {
    if (!game) return;
    try {
      await updateGame({ status: "finished" });
      toast.success("Spiel beendet");
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Fehler");
    }
  }

  const roundQuestion = activeRound ? questionMap.get(activeRound.question) : null;

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" onClick={onBack} className="h-9 w-9" aria-label="Zurück">
          <ArrowLeft className="h-5 w-5" />
        </Button>
        <h2 className="font-semibold text-lg">
          {game.player1_name} vs {game.player2_name}
        </h2>
      </div>

      {/* Score card */}
      <Card className={cn("overflow-hidden", isGameFinished && "ring-2 ring-primary/40")}>
        <CardContent className="p-5">
          <div className="flex items-center justify-around">
            <div className="text-center flex-1">
              <p className="text-sm font-medium text-muted-foreground truncate">
                {game.player1_name}
              </p>
              <p className="text-5xl font-extrabold tabular-nums text-primary mt-1">
                {game.player1_score}
              </p>
            </div>
            <div className="flex flex-col items-center gap-1 px-4">
              <span className="text-2xl font-bold text-muted-foreground">:</span>
              {isGameFinished ? (
                <Badge className="border-0 bg-primary/15 text-primary text-xs">
                  <Trophy className="h-3 w-3 mr-1" />
                  Fertig
                </Badge>
              ) : (
                <Badge variant="secondary" className="text-xs">
                  Runde {Math.min(game.current_round, game.question_count)} / {game.question_count}
                </Badge>
              )}
            </div>
            <div className="text-center flex-1">
              <p className="text-sm font-medium text-muted-foreground truncate">
                {game.player2_name}
              </p>
              <p className="text-5xl font-extrabold tabular-nums text-primary mt-1">
                {game.player2_score}
              </p>
            </div>
          </div>

          {/* Final result */}
          {isGameFinished && (
            <motion.div
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              className="mt-4 rounded-xl bg-primary/10 px-4 py-3 text-center"
            >
              {game.player1_score === game.player2_score ? (
                <p className="font-semibold text-primary">🤝 Unentschieden!</p>
              ) : game.player1_score > game.player2_score ? (
                <p className="font-semibold text-primary">
                  🏆 {game.player1_name} gewinnt!
                </p>
              ) : (
                <p className="font-semibold text-primary">
                  🏆 {game.player2_name} gewinnt!
                </p>
              )}
            </motion.div>
          )}
        </CardContent>
      </Card>

      {/* Active round play area */}
      <AnimatePresence mode="wait">
        {!isGameFinished && !hasMaxRounds && (
          <motion.div
            key="play-area"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
            transition={{ duration: 0.2 }}
          >
            {!activeRound ? (
              <Card className="border-dashed">
                <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
                  <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-2xl">
                    ⚽
                  </div>
                  <div>
                    <p className="font-medium">Bereit für Runde {game.current_round}?</p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Wähle eine Frage aus der Datenbank
                    </p>
                  </div>
                  <Button
                    onClick={() => setPickOpen(true)}
                    disabled={questionsLoading}
                    className="gap-1.5"
                  >
                    <ChevronRight className="h-4 w-4" />
                    Runde {game.current_round} starten
                  </Button>
                  {game.status === "active" && game.current_round > 1 && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-muted-foreground"
                      onClick={handleEndGame}
                    >
                      Spiel jetzt beenden
                    </Button>
                  )}
                </CardContent>
              </Card>
            ) : activeRound.status === "waiting_p1" ? (
              <Card className="border-primary/30 bg-primary/5">
                <CardContent className="space-y-4 p-5">
                  <div className="flex items-center gap-2 text-sm font-medium text-primary">
                    <Target className="h-4 w-4" />
                    Runde {activeRound.round_number} · {game.player1_name} ist dran
                  </div>
                  {roundQuestion && (
                    <>
                      <div className="space-y-1">
                        <div className="flex flex-wrap gap-1.5">
                          <Badge
                            className={cn(
                              "border-0 text-xs",
                              CATEGORY_COLORS[roundQuestion.category],
                            )}
                          >
                            {CATEGORY_EMOJIS[roundQuestion.category]}{" "}
                            {CATEGORY_LABELS[roundQuestion.category]}
                          </Badge>
                          <Badge
                            className={cn(
                              "border-0 text-xs",
                              DIFFICULTY_COLORS[roundQuestion.difficulty],
                            )}
                          >
                            {DIFFICULTY_LABELS[roundQuestion.difficulty]}
                          </Badge>
                        </div>
                        <p className="text-lg font-semibold leading-snug">{roundQuestion.text}</p>
                        {roundQuestion.unit && (
                          <p className="text-sm text-muted-foreground">
                            Einheit: {roundQuestion.unit}
                          </p>
                        )}
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="p1-guess">
                          Schätzung von {game.player1_name}
                        </Label>
                        <div className="flex gap-2">
                          <Input
                            id="p1-guess"
                            type="number"
                            placeholder="Deine Schätzung…"
                            value={p1GuessInput}
                            onChange={(e) => setP1GuessInput(e.target.value)}
                            onKeyDown={(e) => e.key === "Enter" && void handleP1Submit()}
                            className="text-lg font-semibold"
                          />
                          <Button
                            onClick={() => void handleP1Submit()}
                            disabled={submitting || !p1GuessInput}
                          >
                            Fertig
                          </Button>
                        </div>
                      </div>
                    </>
                  )}
                </CardContent>
              </Card>
            ) : activeRound.status === "waiting_p2" ? (
              <Card className="border-primary/30 bg-primary/5">
                <CardContent className="space-y-4 p-5">
                  <div className="flex items-center gap-2 text-sm font-medium text-primary">
                    <Target className="h-4 w-4" />
                    Runde {activeRound.round_number} · {game.player2_name} ist dran
                  </div>
                  {roundQuestion && (
                    <>
                      <div className="space-y-1">
                        <div className="flex flex-wrap gap-1.5">
                          <Badge
                            className={cn(
                              "border-0 text-xs",
                              CATEGORY_COLORS[roundQuestion.category],
                            )}
                          >
                            {CATEGORY_EMOJIS[roundQuestion.category]}{" "}
                            {CATEGORY_LABELS[roundQuestion.category]}
                          </Badge>
                          <Badge
                            className={cn(
                              "border-0 text-xs",
                              DIFFICULTY_COLORS[roundQuestion.difficulty],
                            )}
                          >
                            {DIFFICULTY_LABELS[roundQuestion.difficulty]}
                          </Badge>
                        </div>
                        <p className="text-lg font-semibold leading-snug">{roundQuestion.text}</p>
                        {roundQuestion.unit && (
                          <p className="text-sm text-muted-foreground">
                            Einheit: {roundQuestion.unit}
                          </p>
                        )}
                        <div className="rounded-lg bg-muted px-3 py-2 text-sm text-muted-foreground">
                          {game.player1_name}s Schätzung: <strong>••••</strong>
                        </div>
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="p2-guess">
                          Schätzung von {game.player2_name}
                        </Label>
                        <div className="flex gap-2">
                          <Input
                            id="p2-guess"
                            type="number"
                            placeholder="Deine Schätzung…"
                            value={p2GuessInput}
                            onChange={(e) => setP2GuessInput(e.target.value)}
                            onKeyDown={(e) => e.key === "Enter" && void handleReveal()}
                            className="text-lg font-semibold"
                          />
                          <Button
                            onClick={() => void handleReveal()}
                            disabled={submitting || !p2GuessInput}
                          >
                            Auflösen
                          </Button>
                        </div>
                      </div>
                    </>
                  )}
                </CardContent>
              </Card>
            ) : null}
          </motion.div>
        )}
      </AnimatePresence>

      {/* Past rounds */}
      {!roundsLoading && revealedRounds.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-muted-foreground px-1">
            Gespielte Runden ({revealedRounds.length})
          </h3>
          <motion.div
            className="space-y-2"
            initial="hidden"
            animate="visible"
            variants={{ visible: { transition: { staggerChildren: 0.03 } } }}
          >
            {[...revealedRounds].reverse().map((round) => {
              const q = questionMap.get(round.question);
              const winnerName =
                round.winner === "player1"
                  ? game.player1_name
                  : round.winner === "player2"
                    ? game.player2_name
                    : null;
              return (
                <motion.div
                  key={round.id}
                  variants={{
                    hidden: { opacity: 0, y: 6 },
                    visible: { opacity: 1, y: 0, transition: { duration: 0.18 } },
                  }}
                >
                  <Card className="overflow-hidden">
                    <CardContent className="p-4">
                      <div className="flex items-start gap-3">
                        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-bold">
                          {round.round_number}
                        </div>
                        <div className="flex-1 min-w-0 space-y-2">
                          {q && (
                            <p className="text-sm font-medium leading-snug">{q.text}</p>
                          )}
                          <div className="grid grid-cols-3 gap-2 text-center text-xs">
                            <div className="rounded-lg bg-muted p-2">
                              <p className="text-muted-foreground mb-0.5">{game.player1_name}</p>
                              <p className="font-bold text-sm">{round.player1_guess ?? "–"}</p>
                              {q && round.player1_guess !== null && (
                                <p className="text-muted-foreground">
                                  Δ {Math.abs(round.player1_guess - q.correct_answer)}
                                  {q.unit ? ` ${q.unit}` : ""}
                                </p>
                              )}
                            </div>
                            <div className="flex flex-col items-center justify-center gap-1">
                              {q && (
                                <>
                                  <p className="font-black text-base text-primary">
                                    {q.correct_answer}
                                    {q.unit ? ` ${q.unit}` : ""}
                                  </p>
                                  <p className="text-muted-foreground text-xs">Richtig</p>
                                </>
                              )}
                            </div>
                            <div className="rounded-lg bg-muted p-2">
                              <p className="text-muted-foreground mb-0.5">{game.player2_name}</p>
                              <p className="font-bold text-sm">{round.player2_guess ?? "–"}</p>
                              {q && round.player2_guess !== null && (
                                <p className="text-muted-foreground">
                                  Δ {Math.abs(round.player2_guess - q.correct_answer)}
                                  {q.unit ? ` ${q.unit}` : ""}
                                </p>
                              )}
                            </div>
                          </div>
                          <div className="flex justify-center">
                            {round.winner === "tie" ? (
                              <Badge variant="secondary" className="text-xs">
                                🤝 Unentschieden
                              </Badge>
                            ) : winnerName ? (
                              <Badge className="border-0 bg-primary/15 text-primary text-xs">
                                <Trophy className="h-3 w-3 mr-1" />
                                {winnerName} gewinnt
                              </Badge>
                            ) : null}
                          </div>
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                </motion.div>
              );
            })}
          </motion.div>
        </div>
      )}

      {/* Question picker dialog */}
      <Dialog open={pickOpen} onOpenChange={(o) => !o && setPickOpen(false)}>
        <DialogContent className="max-h-[90dvh] overflow-hidden flex flex-col sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Flag className="h-5 w-5" />
              Frage für Runde {game.current_round} wählen
            </DialogTitle>
          </DialogHeader>

          {/* Filters */}
          <div className="flex gap-2 shrink-0">
            <Select value={catFilter} onValueChange={setCatFilter}>
              <SelectTrigger className="flex-1 h-8 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Alle Kategorien</SelectItem>
                {(Object.keys(CATEGORY_LABELS) as QuestionCategory[]).map((c) => (
                  <SelectItem key={c} value={c}>
                    {CATEGORY_EMOJIS[c]} {CATEGORY_LABELS[c]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={diffFilter} onValueChange={setDiffFilter}>
              <SelectTrigger className="flex-1 h-8 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Alle Schwierigkeiten</SelectItem>
                {(Object.keys(DIFFICULTY_LABELS) as QuestionDifficulty[]).map((d) => (
                  <SelectItem key={d} value={d}>
                    {DIFFICULTY_LABELS[d]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="overflow-y-auto flex-1 -mx-6 px-6 space-y-2 py-1">
            {questionsLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-16 w-full rounded-lg" />
                ))}
              </div>
            ) : filteredQuestions.length === 0 ? (
              <div className="flex flex-col items-center gap-3 py-10 text-center">
                <Search className="h-8 w-8 text-muted-foreground/50" />
                <p className="text-sm text-muted-foreground">
                  Keine verfügbaren Fragen gefunden.
                  {questions.length === 0 && (
                    <> Erstelle zuerst Fragen im Tab „Fragen".</>
                  )}
                </p>
              </div>
            ) : (
              filteredQuestions.map((q) => (
                <button
                  key={q.id}
                  type="button"
                  onClick={() =>
                    setSelectedQuestionId((id) => (id === q.id ? null : q.id))
                  }
                  className={cn(
                    "w-full text-left rounded-xl border p-3 transition-all",
                    selectedQuestionId === q.id
                      ? "border-primary bg-primary/10 ring-1 ring-primary"
                      : "border-border hover:border-primary/50 hover:bg-muted/50",
                  )}
                >
                  <div className="flex items-start gap-2">
                    <span className="text-lg mt-0.5">{CATEGORY_EMOJIS[q.category]}</span>
                    <div className="flex-1 min-w-0 space-y-1">
                      <p className="text-sm font-medium leading-snug">{q.text}</p>
                      <div className="flex gap-1.5 flex-wrap">
                        <Badge
                          className={cn("border-0 text-xs", CATEGORY_COLORS[q.category])}
                        >
                          {CATEGORY_LABELS[q.category]}
                        </Badge>
                        <Badge
                          className={cn("border-0 text-xs", DIFFICULTY_COLORS[q.difficulty])}
                        >
                          {DIFFICULTY_LABELS[q.difficulty]}
                        </Badge>
                      </div>
                    </div>
                  </div>
                </button>
              ))
            )}
          </div>

          <DialogFooter className="gap-2 sm:gap-0 shrink-0">
            <DialogClose asChild>
              <Button variant="outline" disabled={startingRound}>
                Abbrechen
              </Button>
            </DialogClose>
            <Button
              onClick={() => void handleStartRound()}
              disabled={!selectedQuestionId || startingRound}
            >
              {startingRound ? "Wird gestartet…" : "Runde starten"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
