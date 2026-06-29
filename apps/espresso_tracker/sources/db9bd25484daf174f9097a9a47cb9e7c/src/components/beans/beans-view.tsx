import { useState } from "react";
import { AnimatePresence } from "framer-motion";
import { Coffee, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/common/confirm-dialog";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { BeanCard } from "@/components/beans/bean-card";
import { BeanCardSkeleton } from "@/components/beans/bean-card-skeleton";
import { BeanFormDialog } from "@/components/beans/bean-form-dialog";
import type { UseCrudResult } from "@/hooks/use-crud";
import type { Bean, BeanCreateInput, BeanUpdateInput } from "@/client";

interface BeansViewProps {
  beans: UseCrudResult<Bean, BeanCreateInput, BeanUpdateInput>;
}

/** The "Beans" tab: a card grid over bean.list(), with full create/edit/delete. */
export function BeansView({ beans }: BeansViewProps) {
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Bean | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Bean | null>(null);
  const [deleting, setDeleting] = useState(false);

  function openCreate() {
    setEditing(null);
    setFormOpen(true);
  }

  function openEdit(bean: Bean) {
    setEditing(bean);
    setFormOpen(true);
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    setDeleting(true);
    const ok = await beans.remove(deleteTarget.id);
    setDeleting(false);
    if (ok) setDeleteTarget(null);
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">Beans</h2>
          <p className="text-sm text-muted-foreground">
            {beans.loading ? "Loading your shelf…" : `${beans.total} bean${beans.total === 1 ? "" : "s"} on the shelf`}
          </p>
        </div>
        <Button onClick={openCreate}>
          <Plus className="h-4 w-4" /> Add bean
        </Button>
      </div>

      {beans.loading ? (
        <div className="grid gap-3 sm:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <BeanCardSkeleton key={i} />
          ))}
        </div>
      ) : beans.error ? (
        <ErrorState message={beans.error} onRetry={beans.refresh} />
      ) : beans.rows.length === 0 ? (
        <EmptyState
          icon={Coffee}
          title="No beans yet"
          description="Add the beans you're brewing with to track roast dates, roasters, and tasting notes."
          actionLabel="Add your first bean"
          onAction={openCreate}
        />
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          <AnimatePresence initial={false}>
            {beans.rows.map((bean) => (
              <BeanCard key={bean.id} bean={bean} onEdit={() => openEdit(bean)} onDelete={() => setDeleteTarget(bean)} />
            ))}
          </AnimatePresence>
        </div>
      )}

      <BeanFormDialog open={formOpen} onOpenChange={setFormOpen} bean={editing} onCreate={beans.create} onUpdate={beans.update} />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="Delete this bean?"
        description={
          deleteTarget
            ? `"${deleteTarget.name}" will be permanently removed. Any brews logged with it will keep their record but lose the bean link.`
            : ""
        }
        pending={deleting}
        onConfirm={confirmDelete}
      />
    </div>
  );
}
