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
  user_id: string;
  company_name: string;
  product_description: string;
  value_prop: string;
  target_buyer_description: string;
  tone: string;
  signature: string;
  physical_address: string;
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
