import { cn } from "@/lib/utils";

interface ScoreBarProps {
  value: number;
  max?: number;
  className?: string;
}

function scoreColor(pct: number): string {
  if (pct >= 0.7) return "bg-emerald-500";
  if (pct >= 0.4) return "bg-amber-500";
  return "bg-red-400";
}

export function ScoreBar({ value, max = 100, className }: ScoreBarProps) {
  const pct = Math.min(value / max, 1);
  return (
    <div className={cn("flex items-center gap-2", className)}>
      <div className="h-2 w-16 rounded-full bg-muted overflow-hidden">
        <div className={cn("h-full rounded-full transition-all duration-300", scoreColor(pct))} style={{ width: `${pct * 100}%` }} />
      </div>
      <span className="text-xs font-mono tabular-nums">{value.toFixed(0)}</span>
    </div>
  );
}
