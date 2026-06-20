export interface Contact {
  id: string;
  user_id: string;
  business_id: string;
  primary_email: string | null;
  primary_phone: string | null;
  display_name: string;
  pipeline_stage: "lead" | "contacted" | "replied" | "qualified" | "won" | "lost";
  default_automation: "manual" | "semi" | "auto";
  default_sequence_id: string | null;
  unsubscribed_at?: string | null;
  unsubscribe_reason?: string | null;
  created_at: string;
  updated_at: string;
}

export interface UserChannel {
  id: string;
  user_id: string;
  type: "gmail_oauth" | "outlook_oauth" | "smtp" | "whatsapp";
  display_label: string;
  from_email: string;
  oauth_expires_at: string | null;
  oauth_scope: string | null;
  enabled: boolean;
  is_default: boolean;
  last_poll_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface SenderProfile {
  id: string;
  name: string;
  user_id: string;
  company_name: string;
  product_description: string;
  value_prop: string;
  target_buyer_description: string;
  tone: string;
  signature: string;
  physical_address: string;
  // Targeting fields (free text — fed into the AI lead scorer so leads are
  // ranked against this user's specific positioning).
  target_industries: string;
  target_countries: string;
  avoid_countries: string;
  min_deal_size_usd: number | null;
  typical_deal_size_usd: number | null;
  deal_breakers: string;
  competitive_moats: string;
  default_channel_id: string | null;
  catalog_file_name?: string | null;
  catalog_mime_type?: string | null;
  catalog_size_bytes?: number | null;
  catalog_uploaded_at?: string | null;
  updated_at: string;
}

export interface Conversation {
  id: string;
  user_id: string;
  contact_id: string;
  channel_id: string;
  campaign_id: string | null;
  channel_type: string;
  subject: string | null;
  external_thread_id: string | null;
  automation: "manual" | "semi" | "auto";
  status: "active" | "paused" | "closed";
  last_message_at: string | null;
  last_direction: "in" | "out" | null;
  unread: boolean;
  created_at: string;
  updated_at: string;
}

export interface ConversationListRow extends Conversation {
  contact_name: string;
  contact_email: string | null;
  business_id: string;
  business_name: string;
  last_message_snippet: string | null;
  message_count: number;
  last_inbound_sentiment: string | null;
  last_inbound_sentiment_score: number | null;
  last_inbound_sentiment_label: SentimentLevel | null;
  tags: string[];
}

export interface Message {
  id: string;
  conversation_id: string;
  direction: "in" | "out";
  channel_type: string;
  external_id: string | null;
  in_reply_to_external_id: string | null;
  subject: string | null;
  body_text: string | null;
  body_html: string | null;
  ai_generated: boolean;
  ai_model: string | null;
  ai_prompt_version: string | null;
  status: string;
  campaign_id: string | null;
  sequence_step_id: string | null;
  sent_at: string | null;
  received_at: string | null;
  created_at: string;
}

export interface StartConversationResult {
  conversation: Conversation;
  draft_message?: Message;
  draft_error?: string;
}

export interface ConversationDetail {
  conversation: Conversation;
  messages: Message[];
}

// --- Campaigns -------------------------------------------------------

export type CampaignStatus =
  | "draft"
  | "ready"
  | "active"
  | "paused"
  | "completed"
  | "stopped";

export type CampaignContactStatus =
  | "pending"
  | "drafted"
  | "approved"
  | "sent"
  | "replied"
  | "cold"
  | "skipped"
  | "failed";

export interface Campaign {
  id: string;
  user_id: string;
  channel_id: string;
  name: string;
  goal: string;
  status: CampaignStatus;
  positioning_override?: Record<string, unknown> | null;
  sequence_id: string | null;
  market_id: string | null;
  sender_profile_id: string | null;
  on_positive_action: string;
  on_negative_action: string;
  send_pace_per_day: number;
  attach_catalog: boolean;
  start_at: string | null;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CampaignSummary extends Campaign {
  pending_count: number;
  drafted_count: number;
  approved_count: number;
  sent_count: number;
  replied_count: number;
  cold_count: number;
  skipped_count: number;
  failed_count: number;
  total_count: number;
}

export interface CampaignContact {
  campaign_id: string;
  contact_id: string;
  market_id: string | null;
  status: CampaignContactStatus;
  draft_message_id: string | null;
  conversation_id: string | null;
  scheduled_send_at: string | null;
  sent_at: string | null;
  replied_at: string | null;
  skip_reason: string | null;
  added_at: string;
}

export interface CampaignContactRow extends CampaignContact {
  ContactName: string;
  ContactEmail: string | null;
  BusinessName: string;
  DraftSubject: string | null;
  DraftBodyText: string | null;
}

export interface AddContactsResult {
  added: number;
  skipped_overlap: string[];
  skipped_already_in_campaign: number;
}

// --- Bulk add-to-contacts result --------------------------------------

export interface BulkEnsureItem {
  business_id: string;
  name: string;
}

export interface BulkEnsureResult {
  added: BulkEnsureItem[];
  already_existed: BulkEnsureItem[];
  no_email: BulkEnsureItem[];
}

// --- Sequences -------------------------------------------------------

export type SequenceTrigger = "no_reply" | "any_reply" | "positive_reply" | "always";
export type SequenceAction =
  | "send_message"
  | "mark_cold"
  | "notify_user"
  | "advance_stage";

export interface Sequence {
  id: string;
  user_id: string;
  name: string;
  description: string;
  is_template: boolean;
  created_at: string;
  updated_at: string;
}

export interface SequenceStep {
  id?: string;
  sequence_id?: string;
  step_number: number;
  wait_days: number;
  trigger: SequenceTrigger;
  action: SequenceAction;
  prompt_override: string | null;
  auto_send: boolean;
}

export interface SequenceWithSteps extends Sequence {
  steps: SequenceStep[];
  active_runs_count: number;
}

// --- CRM tasks + notes ----------------------------------------------

export interface Task {
  id: string;
  user_id: string;
  contact_id: string | null;
  conversation_id: string | null;
  title: string;
  body: string;
  due_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface Note {
  id: string;
  user_id: string;
  contact_id: string;
  body: string;
  created_at: string;
}

// --- Contact groups ---------------------------------------------------

export interface ContactGroup {
  id: string;
  user_id: string;
  name: string;
  sender_profile_id: string | null;
  member_count: number;
  created_at: string;
  updated_at: string;
}

// --- Email Groups -----------------------------------------------------
// An Email Group is a campaign + its attached follow-up steps, presented as
// one thing. These shapes mirror the backend group facade.

// Sentiment is a continuous score (-1..1) surfaced as one of five levels.
export type SentimentLevel =
  | "very_negative"
  | "negative"
  | "neutral"
  | "positive"
  | "very_positive";

// Status facet keys (status + derived buckets).
export type StatusFacet = "replied" | "no_reply" | "cold" | "advanced";

// EmailFacets is the multi-select filter: OR within a facet, AND across facets.
export interface EmailFacets {
  sentiment: SentimentLevel[];
  tags: string[];
  status: StatusFacet[];
}

export interface GroupSummary extends CampaignSummary {
  contact_group_id: string | null;
  contact_group_name: string;
  brand_id: string | null;
  brand_name: string;
  sender_email: string;
}

export interface GroupDetail extends GroupSummary {
  steps: SequenceStep[];
  facet_counts: Record<string, number>;
  notifications: Task[];
}

export interface GroupEmail {
  contact_id: string;
  contact_name: string;
  contact_email: string | null;
  business_name: string;
  conversation_id: string | null;
  subject: string | null;
  status: CampaignContactStatus;
  last_direction: "in" | "out" | null;
  last_message_at: string | null;
  sentiment: string | null;
  sentiment_score: number | null;
  sentiment_label: SentimentLevel | null;
  tags: string[];
  unread: boolean;
  has_pending_draft: boolean;
  scheduled_send_at: string | null;
  current_step: number | null;
  next_run_at: string | null;
}

export interface FunnelStep {
  step: number;
  label: string;
  sent: number;
  pct: number;
}

export interface UpcomingDay {
  date: string;
  cold: number;
  followup: number;
}

export interface CadenceStats {
  total_contacts: number;
  funnel: FunnelStep[];
  exits: Record<string, number>;
  completion_pct: number;
  upcoming: UpcomingDay[];
}

// ConvTag is a stored intent tag with its provenance (ai|manual).
export interface ConvTag {
  tag: string;
  source: "ai" | "manual";
  confidence: number | null;
}

// PlaybookEntry is one authored per-tag reply instruction for a group.
export interface PlaybookEntry {
  tag: string;
  instruction: string;
}

// AssistantAction is a write the agent proposes (the user confirms it).
export interface AssistantAction {
  type: "edit_drafts" | "add_playbook" | "remove_playbook";
  scope?: "cold" | "reply" | "reply_positive" | "reply_negative" | "all";
  instruction?: string;
  tag?: string;
}

// AssistantMessage is one turn in the group's draft-assistant chat thread.
export interface AssistantMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  proposed_action?: AssistantAction | null;
  status: "sent" | "proposed" | "applied" | "dismissed";
  created_at: string;
}

// BrandLesson is a remembered per-brand reply instruction, matched by tags +
// sentiment so it only applies to the same kind of buyer reply.
export interface BrandLesson {
  id: string;
  instruction: string;
  match_tags: string[];
  match_sentiment: string;
  created_at: string;
}
