import { motion } from "framer-motion";
import { Calendar, MoreVertical, Pencil, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { formatDate } from "@/lib/format";
import type { Bean } from "@/client";

interface BeanCardProps {
  bean: Bean;
  onEdit: () => void;
  onDelete: () => void;
}

export function BeanCard({ bean, onEdit, onDelete }: BeanCardProps) {
  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -8, transition: { duration: 0.15 } }}
      transition={{ duration: 0.2 }}
    >
      <Card className="h-full">
        <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
          <div className="space-y-1.5">
            <h3 className="font-semibold leading-tight">{bean.name}</h3>
            {bean.roaster && <Badge variant="secondary">{bean.roaster}</Badge>}
          </div>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0" aria-label={`Actions for ${bean.name}`}>
                <MoreVertical className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onSelect={onEdit}>
                <Pencil className="h-4 w-4" /> Edit
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={onDelete} className="text-destructive focus:text-destructive">
                <Trash2 className="h-4 w-4" /> Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </CardHeader>
        <CardContent className="space-y-2">
          {bean.roast_date && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Calendar className="h-3.5 w-3.5" /> Roasted {formatDate(bean.roast_date)}
            </p>
          )}
          {bean.notes ? (
            <p className="line-clamp-3 text-sm text-muted-foreground">{bean.notes}</p>
          ) : (
            <p className="text-sm text-muted-foreground/60 italic">No notes yet.</p>
          )}
        </CardContent>
      </Card>
    </motion.div>
  );
}
