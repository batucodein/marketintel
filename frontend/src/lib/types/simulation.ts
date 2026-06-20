import type { SequenceStep } from "./outreach";

export interface PersonaOption {
  key: string;
  label: string;
}

export interface TranscriptMessage {
  who: "you" | "them" | "event";
  day: number;
  subject?: string;
  body: string;
  sentiment?: string;
  sentiment_level?: import("./outreach").SentimentLevel;
  tags?: string[];
}

export interface SimulationGrade {
  score: number;
  human_feel: number;
  correctness: number;
  notes: string;
}

export interface PendingDraft {
  kind: "cold" | "followup" | "reply";
  subject: string;
  body: string;
  step: number;
}

export interface SimulationLead {
  id: string;
  persona: string;
  display_name: string;
  business_id: string | null;
  transcript: TranscriptMessage[];
  outcome: string;
  grade?: SimulationGrade | null;
  created_at: string;
  // Live interactive state.
  state: "active" | "awaiting_approval" | "snoozed" | "done";
  current_step: number;
  touch: number;
  last_direction?: string | null;
  last_sentiment?: string | null;
  next_day: number;
  snooze_until_day?: number | null;
  replied_once: boolean;
  pending_draft?: PendingDraft | null;
}

export interface SimulationSummary {
  lead_count: number;
  outcomes: Record<string, number>;
  avg_score: number;
  avg_human_feel: number;
  sentiment_accuracy?: number;
}

export interface Simulation {
  id: string;
  name: string;
  market_id: string | null;
  sender_profile_id: string | null;
  steps: SequenceStep[];
  status: "running" | "paused" | "done" | "failed";
  summary?: SimulationSummary | null;
  error?: string | null;
  created_at: string;
  completed_at: string | null;
  leads?: SimulationLead[];
  // Interactive config / live state.
  mode: "interactive" | "quick";
  virtual_day: number;
  playbook: Record<string, string>;
  on_positive_action: string;
  on_negative_action: string;
  include_lessons: boolean;
  pending_count: number;
}
