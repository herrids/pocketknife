import { motion } from "framer-motion";
import { Coffee, MoreVertical, Pencil, Repeat, Thermometer, Timer, Trash2 } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { StarRating } from "@/components/brews/star-rating";
import { formatRelative, formatSeconds, humanize } from "@/lib/format";
import type { Bean, Brew } from "@/client";

interface BrewCardProps {
  brew: Brew;
  bean: Bean | undefined;
  onEdit: () => void;
  onDelete: () => void;
}

export function BrewCard({ brew, bean, onEdit, onDelete }: BrewCardProps) {
  const ratio = brew.output_g !== null && brew.dose_g > 0 ? brew.output_g / brew.dose_g : null;

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: -8, transition: { duration: 0.15 } }}
      transition={{ duration: 0.2 }}
    >
      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0 pb-3">
          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              <Badge variant={brew.method === "espresso" ? "default" : "secondary"}>{humanize(brew.method)}</Badge>
              {brew.inverted && <Badge variant="outline">Inverted</Badge>}
            </div>
            <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <Coffee className="h-3.5 w-3.5" />
              {bean ? bean.name : "No bean selected"}
            </p>
          </div>
          <div className="flex items-center gap-1">
            <StarRating value={brew.rating} readOnly size="sm" />
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0" aria-label="Brew actions">
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
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <Stat label="Dose" value={`${brew.dose_g.toFixed(1)} g`} />
            <Stat label="Yield" value={brew.output_g !== null ? `${brew.output_g.toFixed(1)} g` : "—"} />
            <Stat label="Ratio" value={ratio !== null ? `1:${ratio.toFixed(1)}` : "—"} icon={Repeat} />
            <Stat label="Time" value={brew.brew_time_s !== null ? formatSeconds(brew.brew_time_s) : "—"} icon={Timer} />
          </div>

          {(brew.water_temp_c !== null || brew.grind_setting) && (
            <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
              {brew.water_temp_c !== null && (
                <span className="flex items-center gap-1">
                  <Thermometer className="h-3.5 w-3.5" /> {brew.water_temp_c.toFixed(1)}°C
                </span>
              )}
              {brew.grind_setting && <span>Grind: {brew.grind_setting}</span>}
            </div>
          )}

          {brew.notes && <p className="line-clamp-2 text-sm text-muted-foreground">{brew.notes}</p>}

          <p className="text-right text-xs text-muted-foreground">Logged {formatRelative(brew.created_at)}</p>
        </CardContent>
      </Card>
    </motion.div>
  );
}

function Stat({ label, value, icon: Icon }: { label: string; value: string; icon?: LucideIcon }) {
  return (
    <div className="rounded-lg bg-muted/50 px-2.5 py-2">
      <div className="flex items-center gap-1 text-[11px] uppercase tracking-wide text-muted-foreground">
        {Icon && <Icon className="h-3 w-3" />}
        {label}
      </div>
      <div className="text-sm font-medium">{value}</div>
    </div>
  );
}
