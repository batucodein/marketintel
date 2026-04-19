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
  default_channel_id: string | null;
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
