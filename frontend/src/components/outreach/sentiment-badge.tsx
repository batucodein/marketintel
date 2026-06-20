import { Badge } from "@/components/ui/badge";
import type { SentimentLevel } from "@/lib/types/outreach";
import { SENTIMENT_META, levelFromLegacy } from "./intent-tags";

// SentimentBadge renders the 5-level sentiment as an intensity-coloured chip.
// Pass the derived label; falls back to the legacy 3-value string for rows that
// predate scoring. Renders nothing when there's no sentiment.
export function SentimentBadge({
  label,
  legacy,
  score,
  className = "",
}: {
  label?: SentimentLevel | null;
  legacy?: string | null;
  score?: number | null;
  className?: string;
}) {
  const level = label ?? levelFromLegacy(legacy);
  if (!level) return null;
  const meta = SENTIMENT_META[level];
  return (
    <Badge
      variant="outline"
      title={score != null ? `score ${score.toFixed(2)}` : meta.label}
      className={`text-[10px] whitespace-nowrap shrink-0 ${meta.style} ${className}`}
    >
      {meta.icon} {meta.label}
    </Badge>
  );
}
