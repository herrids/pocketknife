import { useMemo, useState } from "react";
import { AnimatePresence } from "framer-motion";
import { Coffee, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { BrewCard } from "@/components/brews/brew-card";
import { BrewCardSkeleton } from "@/components/brews/brew-card-skeleton";
import { BrewFormDialog } from "@/components/brews/brew-form-dialog";
import type { UseCrudResult } from "@/hooks/use-crud";
import type { Bean, Brew, BrewCreateInput, BrewUpdateInput } from "@/client";

interface BrewsViewProps {
  brews: UseCrudResult<Brew, BrewCreateInput, BrewUpdateInput>;
  beans: Bean[];
}

/** The "Brews" tab: a feed of brew.list() rows over a shot-ticket card, with full create/edit/delete. */
export function BrewsView({ brews, beans }: BrewsViewProps) {
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Brew | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Brew | null>(null);
  const [deleting, setDeleting] = useState(false);

  const beanById = useMemo(() => new Map(beans.map((bean) => [bean.id, bean])), [beans]);

  function openCreate() {
    setEditing(null);
    setFormOpen(true);
  }

  function openEdit(brew: Brew) {
    setEditing(brew);
    setFormOpen(true);
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    const ok = await brews.remove(deleteTarget.id);
    setDeleting(false);
    if (ok) setDeleteTarget(null);
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">Brews</h2>
          <p className="text-sm text-muted-foreground">
            {brews.loading ? "Loading your log…" : `${brews.total} brew${brews.total === 1 ? "" : "s"} logged`}
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4" /> Log a brew
        </Button>
      </div>

      {brews.loading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <BrewCardSkeleton key={i} />
          ))}
        </div>
      ) : brews.error ? (
        <ErrorState message={brews.error} onRetry={brews.refresh} />
      ) : brews.rows.length === 0 ? (
        <EmptyState
          icon={Coffee}
          title="No brews logged yet"
          description="Log your first cup to start dialing in your recipe and tracking what works."
          actionLabel="Log your first brew"
          onAction={openCreate}
        />
      ) : (
        <div className="space-y-3">
          <AnimatePresence initial={false}>
            {brews.rows.map((brew) => (
              <BrewCard
                key={brew.id}
                brew={brew}
                bean={brew.bean ? beanById.get(brew.bean) : undefined}
                onEdit={() => openEdit(brew)}
                onDelete={() => setDeleteTarget(brew)}
              />
            ))}
          </AnimatePresence>
        </div>
      )}

      <BrewFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        brew={editing}
        beans={beans}
        onCreate={brews.create}
        onUpdate={brews.update}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="Delete this brew?"
        description="This brew record will be permanently removed. This can't be undone."
        pending={deleting}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
