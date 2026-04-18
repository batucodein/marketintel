"use client";

import type { DataQuality } from "@/lib/types/data-quality";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Badge } from "@/components/ui/badge";
import { Info } from "lucide-react";
import { cn } from "@/lib/utils";

const STATUS_STYLES: Record<string, { bg: string; text: string }> = {
  available: { bg: "bg-emerald-100", text: "text-emerald-700" },
  partial: { bg: "bg-amber-100", text: "text-amber-700" },
  unavailable: { bg: "bg-red-100", text: "text-red-700" },
  not_configured: { bg: "bg-gray-100", text: "text-gray-500" },
};

interface DataQualityPanelProps {
  dataQuality: DataQuality;
  warnings?: string[];
}

export function DataQualityPanel({ dataQuality, warnings }: DataQualityPanelProps) {
  const pct = Math.round(dataQuality.overall_completeness * 100);

  return (
    <div className="space-y-3">
      {/* Completeness bar */}
      <div className="flex items-center gap-3">
        <span className="text-xs font-medium text-muted-foreground whitespace-nowrap">
          Data Quality
        </span>
        <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
          <div
            className={cn(
              "h-full rounded-full transition-all duration-500",
              pct >= 70 ? "bg-emerald-500" : pct >= 40 ? "bg-amber-500" : "bg-red-400",
            )}
            style={{ width: `${pct}%` }}
          />
        </div>
        <span className="text-xs font-mono tabular-nums">{pct}%</span>
      </div>

      {/* Source badges */}
      <div className="flex flex-wrap gap-2">
        {dataQuality.sources.map((src) => {
          const styles = STATUS_STYLES[src.status] || STATUS_STYLES.not_configured;
          return (
            <Tooltip key={src.source}>
              <TooltipTrigger>
                <Badge
                  variant="outline"
                  className={cn("text-xs cursor-default", styles.bg, styles.text)}
                >
                  {src.source.replace(/_/g, " ")}
                  {src.items != null && src.items > 0 && ` (${src.items})`}
                </Badge>
              </TooltipTrigger>
              <TooltipContent>
                <p className="text-xs">
                  {src.status}{src.message ? `: ${src.message}` : ""}
                </p>
              </TooltipContent>
            </Tooltip>
          );
        })}
      </div>

      {/* Missing data note */}
      {dataQuality.missing_data_note && (
        <div className="flex items-start gap-2 rounded-md bg-slate-100 px-3 py-2">
          <Info className="h-4 w-4 text-slate-500 mt-0.5 shrink-0" />
          <p className="text-xs text-slate-600">
            Some data sources were unavailable. Results may be less comprehensive.
          </p>
        </div>
      )}

      {/* Warnings */}
      {warnings && warnings.length > 0 && (
        <div className="rounded-md bg-amber-50 border border-amber-200 px-3 py-2">
          <ul className="space-y-1">
            {warnings.map((w, i) => (
              <li key={i} className="text-xs text-amber-700">{w}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
