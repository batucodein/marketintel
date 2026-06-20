"use client";

import type { EmailFacets, SentimentLevel, StatusFacet } from "@/lib/types/outreach";
import { INTENT_TAGS, SENTIMENT_ORDER, SENTIMENT_META, tagLabel } from "./intent-tags";

const STATUS_FACETS: { value: StatusFacet; label: string }[] = [
  { value: "replied", label: "Replied" },
  { value: "no_reply", label: "No reply" },
  { value: "cold", label: "Cold" },
  { value: "advanced", label: "Advanced" },
];

// FacetChip is the shared filter pill — exported so the Simulation page can
// reuse the exact same control for its preview filtering.
export function FacetChip({
  label,
  count,
  active,
  onClick,
}: {
  label: string;
  count?: number;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        "rounded-full border px-3 py-1 text-xs transition cursor-pointer " +
        (active
          ? "border-primary bg-primary/10 text-primary font-medium"
          : "border-border text-muted-foreground hover:border-primary/50")
      }
    >
      {label}
      {count != null && <span className="ml-1 opacity-60">{count}</span>}
    </button>
  );
}

// FacetFilterBar is a controlled multi-select filter: OR within a facet, AND
// across facets. Intent tags are only shown when present in the group (count>0)
// or currently selected, to keep the bar uncluttered.
export function FacetFilterBar({
  facets,
  counts,
  onChange,
}: {
  facets: EmailFacets;
  counts: Record<string, number>;
  onChange: (next: EmailFacets) => void;
}) {
  function toggle<T extends string>(list: T[], v: T): T[] {
    return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
  }

  const visibleTags = INTENT_TAGS.filter(
    (t) => (counts[t.value] ?? 0) > 0 || facets.tags.includes(t.value),
  );

  const anyActive =
    facets.sentiment.length > 0 || facets.tags.length > 0 || facets.status.length > 0;

  return (
    <div className="space-y-2">
      {/* Sentiment */}
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-[11px] uppercase tracking-wide text-muted-foreground/70 w-16 shrink-0">
          Sentiment
        </span>
        {SENTIMENT_ORDER.map((lvl: SentimentLevel) => (
          <FacetChip
            key={lvl}
            label={`${SENTIMENT_META[lvl].icon} ${SENTIMENT_META[lvl].label}`}
            count={counts[lvl]}
            active={facets.sentiment.includes(lvl)}
            onClick={() => onChange({ ...facets, sentiment: toggle(facets.sentiment, lvl) })}
          />
        ))}
      </div>

      {/* Status */}
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-[11px] uppercase tracking-wide text-muted-foreground/70 w-16 shrink-0">
          Status
        </span>
        {STATUS_FACETS.map((s) => (
          <FacetChip
            key={s.value}
            label={s.label}
            count={counts[s.value]}
            active={facets.status.includes(s.value)}
            onClick={() => onChange({ ...facets, status: toggle(facets.status, s.value) })}
          />
        ))}
      </div>

      {/* Intent tags (only those present in the group) */}
      {visibleTags.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-[11px] uppercase tracking-wide text-muted-foreground/70 w-16 shrink-0">
            Intent
          </span>
          {visibleTags.map((t) => (
            <FacetChip
              key={t.value}
              label={tagLabel(t.value)}
              count={counts[t.value]}
              active={facets.tags.includes(t.value)}
              onClick={() => onChange({ ...facets, tags: toggle(facets.tags, t.value) })}
            />
          ))}
        </div>
      )}

      {anyActive && (
        <button
          type="button"
          onClick={() => onChange({ sentiment: [], tags: [], status: [] })}
          className="text-[11px] text-muted-foreground hover:text-foreground underline underline-offset-2 cursor-pointer"
        >
          Clear filters
        </button>
      )}
    </div>
  );
}
