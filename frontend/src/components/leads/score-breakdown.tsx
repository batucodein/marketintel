"use client";

import { ResponsiveContainer, RadarChart, PolarGrid, PolarAngleAxis, PolarRadiusAxis, Radar } from "recharts";
import type { BusinessWithRelevance } from "@/lib/types/business";
import { Info } from "lucide-react";

interface ScoreBreakdownProps {
  lead: BusinessWithRelevance;
}

const DIMENSIONS = [
  { key: "deal_size_potential", label: "Volume (30%)" },
  { key: "purchase_likelihood", label: "Activity (25%)" },
  { key: "accessibility_score", label: "Reachability (25%)" },
  { key: "fit_score", label: "Fit (15%)" },
  { key: "urgency_score", label: "Recency (5%)" },
] as const;

export function ScoreBreakdown({ lead }: ScoreBreakdownProps) {
  const data = DIMENSIONS.map(({ key, label }) => ({
    dimension: label,
    value: (lead[key] as number | null | undefined) ?? 0,
  }));

  const hasAnyScore = data.some((d) => d.value > 0);

  if (!hasAnyScore) {
    return (
      <div className="flex items-center gap-2 rounded-md bg-slate-100 px-3 py-2">
        <Info className="h-4 w-4 text-slate-500 shrink-0" />
        <p className="text-xs text-slate-600">No sub-scores available yet.</p>
      </div>
    );
  }

  return (
    <div className="w-full h-56">
      <ResponsiveContainer width="100%" height="100%">
        <RadarChart cx="50%" cy="50%" outerRadius="70%" data={data}>
          <PolarGrid stroke="#e2e8f0" />
          <PolarAngleAxis
            dataKey="dimension"
            tick={{ fontSize: 11, fill: "#64748b" }}
          />
          <PolarRadiusAxis
            angle={90}
            domain={[0, 100]}
            tick={{ fontSize: 9, fill: "#94a3b8" }}
            tickCount={5}
          />
          <Radar
            dataKey="value"
            stroke="#2563eb"
            fill="#3b82f6"
            fillOpacity={0.25}
            strokeWidth={2}
          />
        </RadarChart>
      </ResponsiveContainer>
    </div>
  );
}
