"use client";

import type { BusinessWithRelevance } from "@/lib/types/business";
import { CheckCircle2, XCircle, Lightbulb, Brain } from "lucide-react";

interface ScoringRationaleProps {
  lead: BusinessWithRelevance;
}

export function ScoringRationale({ lead }: ScoringRationaleProps) {
  const hasContent = lead.scoring_rationale || lead.strengths?.length || lead.weaknesses?.length || lead.recommended_approach;

  if (!hasContent) return null;

  return (
    <div className="space-y-4 pt-2">
      {/* AI Rationale */}
      {lead.scoring_rationale && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
            <Brain className="h-3.5 w-3.5" />
            AI Rationale
          </h4>
          <p className="text-sm text-muted-foreground leading-relaxed">
            {lead.scoring_rationale}
          </p>
        </div>
      )}

      {/* Strengths */}
      {lead.strengths && lead.strengths.length > 0 && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wider text-emerald-700 flex items-center gap-1.5">
            <CheckCircle2 className="h-3.5 w-3.5" />
            Strengths
          </h4>
          <ul className="space-y-1">
            {lead.strengths.map((s, i) => (
              <li key={i} className="flex items-start gap-2 text-sm">
                <span className="text-emerald-500 mt-0.5 shrink-0">+</span>
                <span className="text-muted-foreground">{s}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Weaknesses */}
      {lead.weaknesses && lead.weaknesses.length > 0 && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wider text-red-700 flex items-center gap-1.5">
            <XCircle className="h-3.5 w-3.5" />
            Weaknesses
          </h4>
          <ul className="space-y-1">
            {lead.weaknesses.map((w, i) => (
              <li key={i} className="flex items-start gap-2 text-sm">
                <span className="text-red-400 mt-0.5 shrink-0">-</span>
                <span className="text-muted-foreground">{w}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Recommended approach */}
      {lead.recommended_approach && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wider text-blue-700 flex items-center gap-1.5">
            <Lightbulb className="h-3.5 w-3.5" />
            Recommended Approach
          </h4>
          <p className="text-sm text-muted-foreground leading-relaxed">
            {lead.recommended_approach}
          </p>
        </div>
      )}
    </div>
  );
}
