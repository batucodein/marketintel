// Intent-tag vocabulary, mirrored from the backend domain/intent.go controlled
// vocabulary. Grouped by class for colouring + the manual tag picker.
import type { SentimentLevel } from "@/lib/types/outreach";

export type TagClass = "buying" | "objection" | "routing";

export interface IntentTagMeta {
  value: string;
  label: string;
  class: TagClass;
}

export const INTENT_TAGS: IntentTagMeta[] = [
  // Buying signals
  { value: "price_requested", label: "Price asked", class: "buying" },
  { value: "info_requested", label: "Info asked", class: "buying" },
  { value: "sample_requested", label: "Sample asked", class: "buying" },
  { value: "meeting_requested", label: "Meeting", class: "buying" },
  { value: "moq_question", label: "MOQ question", class: "buying" },
  { value: "shipping_question", label: "Shipping", class: "buying" },
  { value: "certification_question", label: "Certification", class: "buying" },
  { value: "ready_to_order", label: "Ready to order", class: "buying" },
  // Objections
  { value: "price_objection", label: "Price objection", class: "objection" },
  { value: "timing_objection", label: "Timing", class: "objection" },
  { value: "has_supplier", label: "Has supplier", class: "objection" },
  { value: "not_interested", label: "Not interested", class: "objection" },
  // Routing / compliance
  { value: "unsubscribe", label: "Unsubscribe", class: "routing" },
  { value: "out_of_office", label: "Out of office", class: "routing" },
  { value: "wrong_contact", label: "Wrong contact", class: "routing" },
  { value: "referral", label: "Referral", class: "routing" },
];

const TAG_BY_VALUE = new Map(INTENT_TAGS.map((t) => [t.value, t]));

export function tagLabel(value: string): string {
  return TAG_BY_VALUE.get(value)?.label ?? value.replace(/_/g, " ");
}

// Harmonic, theme-aligned chip styles per class (work in light + dark).
const CLASS_STYLE: Record<TagClass, string> = {
  buying: "border-emerald-300/60 text-emerald-700 bg-emerald-50 dark:bg-emerald-500/10 dark:text-emerald-300",
  objection: "border-amber-300/60 text-amber-700 bg-amber-50 dark:bg-amber-500/10 dark:text-amber-300",
  routing: "border-slate-300/60 text-slate-600 bg-slate-50 dark:bg-slate-500/10 dark:text-slate-300",
};

export function tagStyle(value: string): string {
  const cls = TAG_BY_VALUE.get(value)?.class ?? "routing";
  return CLASS_STYLE[cls];
}

// --- Sentiment levels -------------------------------------------------------

export const SENTIMENT_ORDER: SentimentLevel[] = [
  "very_negative",
  "negative",
  "neutral",
  "positive",
  "very_positive",
];

export const SENTIMENT_META: Record<
  SentimentLevel,
  { label: string; icon: string; style: string }
> = {
  very_negative: {
    label: "Very negative",
    icon: "▼▼",
    style: "border-red-400/60 text-red-700 bg-red-50 dark:bg-red-500/15 dark:text-red-300",
  },
  negative: {
    label: "Negative",
    icon: "▼",
    style: "border-amber-400/60 text-amber-700 bg-amber-50 dark:bg-amber-500/15 dark:text-amber-300",
  },
  neutral: {
    label: "Neutral",
    icon: "○",
    style: "border-slate-300/60 text-slate-600 bg-slate-50 dark:bg-slate-500/15 dark:text-slate-300",
  },
  positive: {
    label: "Positive",
    icon: "▲",
    style: "border-emerald-300/60 text-emerald-700 bg-emerald-50 dark:bg-emerald-500/15 dark:text-emerald-300",
  },
  very_positive: {
    label: "Very positive",
    icon: "▲▲",
    style: "border-emerald-400/70 text-emerald-800 bg-emerald-100 dark:bg-emerald-500/25 dark:text-emerald-200",
  },
};

// Fallback for legacy 3-value sentiment strings (rows not yet re-classified).
export function levelFromLegacy(s: string | null | undefined): SentimentLevel | null {
  switch (s) {
    case "positive":
      return "positive";
    case "negative":
      return "negative";
    case "neutral":
      return "neutral";
    default:
      return null;
  }
}
