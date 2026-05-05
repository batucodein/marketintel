"use client";

import { Badge } from "@/components/ui/badge";
import { AlertTriangle } from "lucide-react";
import type { BusinessWithRelevance } from "@/lib/types/business";

interface Props {
  lead: BusinessWithRelevance;
}

// Mapping between the score-breakdown's per-dimension fields and the
// dimension_completeness JSON keys that Phase B scoring writes.
const DIMS: Array<{
  scoreKey: keyof BusinessWithRelevance;
  completenessKey: string;
  label: string;
}> = [
  { scoreKey: "deal_size_potential", completenessKey: "deal_size", label: "Deal size potential" },
  { scoreKey: "purchase_likelihood", completenessKey: "purchase_likelihood", label: "Purchase likelihood" },
  { scoreKey: "accessibility_score", completenessKey: "accessibility", label: "Accessibility" },
  { scoreKey: "fit_score", completenessKey: "fit", label: "Fit" },
  { scoreKey: "urgency_score", completenessKey: "urgency", label: "Urgency" },
];

export function DimensionBars({ lead }: Props) {
  const completeness = lead.dimension_completeness ?? {};
  const missing = lead.missing_fields ?? [];

  const hasAnyScore = DIMS.some((d) => {
    const v = lead[d.scoreKey] as number | null | undefined;
    return typeof v === "number" && v > 0;
  });
  if (!hasAnyScore) return null;

  return (
    <div className="space-y-2">
      <h4 className="text-xs font-semibold text-muted-foreground uppercase">
        Score breakdown · data quality
      </h4>
      <div className="space-y-1.5">
        {DIMS.map((d) => {
          const score = (lead[d.scoreKey] as number | null | undefined) ?? 0;
          const dq = completeness[d.completenessKey];
          const dqPct = typeof dq === "number" ? Math.round(dq * 100) : null;
          const lowQuality = typeof dq === "number" && dq < 0.4;
          return (
            <div key={d.completenessKey} className="grid grid-cols-[1fr_auto_120px_60px] gap-2 items-center text-xs">
              <span className="text-foreground">{d.label}</span>
              <span className="font-mono font-medium tabular-nums w-8 text-right">{score}</span>
              <div className="flex items-center gap-1">
                <div className="flex-1 h-1.5 bg-muted rounded overflow-hidden">
                  <div
                    className={`h-full ${lowQuality ? "bg-amber-500" : "bg-blue-600"}`}
                    style={{ width: `${score}%` }}
                  />
                </div>
              </div>
              <span
                className={`text-[10px] font-mono tabular-nums text-right ${
                  lowQuality ? "text-amber-700 font-semibold" : "text-muted-foreground"
                }`}
              >
                {dqPct === null ? "—" : `data ${dqPct}%`}
              </span>
            </div>
          );
        })}
      </div>

      {missing.length > 0 && (
        <div className="mt-2 flex items-start gap-2 rounded-md bg-amber-50 border border-amber-200 px-2 py-1.5">
          <AlertTriangle className="h-3.5 w-3.5 text-amber-700 mt-0.5 shrink-0" />
          <div className="flex-1 min-w-0">
            <p className="text-[11px] font-medium text-amber-900 mb-0.5">
              Missing fields lowered some scores
            </p>
            <div className="flex flex-wrap gap-1">
              {missing.map((m) => (
                <Badge key={m} variant="outline" className="text-[10px] py-0 px-1.5 border-amber-300 text-amber-800">
                  {m}
                </Badge>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
