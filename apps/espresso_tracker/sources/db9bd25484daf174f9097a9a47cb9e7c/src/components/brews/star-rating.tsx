import { Star } from "lucide-react";

import { cn } from "@/lib/utils";

interface StarRatingProps {
  value: number | null;
  onChange?: (value: number | null) => void;
  readOnly?: boolean;
  size?: "sm" | "md";
}

const SIZE_CLASS: Record<"sm" | "md", string> = {
  sm: "h-3.5 w-3.5",
  md: "h-5 w-5",
};

const STARS = [1, 2, 3, 4, 5];

/** A 1–5 star rating: clickable picker in forms, plain display on cards. Clicking the active star clears it. */
export function StarRating({ value, onChange, readOnly = false, size = "md" }: StarRatingProps) {
  const starClass = SIZE_CLASS[size];

  if (readOnly && !value) {
    return <span className="text-xs text-muted-foreground">Not rated</span>;
  }

  return (
    <div className="flex items-center gap-0.5" role={readOnly ? undefined : "radiogroup"} aria-label="Rating">
      {STARS.map((star) => {
        const filled = value !== null && star <= value;
        const icon = (
          <Star className={cn(starClass, filled ? "fill-primary text-primary" : "text-muted-foreground/40")} />
        );

        if (readOnly) {
          return <span key={star}>{icon}</span>;
        }

        return (
          <button
            key={star}
            type="button"
            role="radio"
            aria-checked={value === star}
            aria-label={`${star} star${star === 1 ? "" : "s"}`}
            onClick={() => onChange?.(value === star ? null : star)}
            className="rounded p-0.5 transition-transform hover:scale-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {icon}
          </button>
        );
      })}
    </div>
  );
}
