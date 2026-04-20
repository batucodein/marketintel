import { apiFetch } from "./client";
import type {
  Contact,
  UserChannel,
  SenderProfile,
  ConversationListRow,
  ConversationDetail,
  StartConversationResult,
  Message,
} from "../types/outreach";

// --- Channels ----------------------------------------------------------

export function listChannels(): Promise<UserChannel[]> {
  return apiFetch("/outreach/channels");
}

export function deleteChannel(id: string): Promise<void> {
  return apiFetch(`/outreach/channels/${id}`, { method: "DELETE" });
}

export function setDefaultChannel(id: string): Promise<void> {
  return apiFetch(`/outreach/channels/${id}/default`, { method: "POST" });
}

export function getGmailAuthURL(): Promise<{ auth_url: string }> {
  return apiFetch("/outreach/channels/gmail/auth-url");
}

// --- Sender profile ---------------------------------------------------

export function getSenderProfile(): Promise<SenderProfile> {
  return apiFetch("/outreach/sender-profile");
}

export function updateSenderProfile(p: Partial<SenderProfile>): Promise<SenderProfile> {
  return apiFetch("/outreach/sender-profile", {
    method: "PUT",
    body: JSON.stringify(p),
  });
}

// --- Contacts ---------------------------------------------------------

export function listContacts(
  stage?: string,
  page = 1,
  pageSize = 50,
): Promise<{ contacts: Contact[]; total: number; page: number; page_size: number }> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (stage) params.set("stage", stage);
  return apiFetch(`/outreach/contacts?${params}`);
}

export function getContact(id: string): Promise<Contact> {
  return apiFetch(`/outreach/contacts/${id}`);
}

export function updateContact(id: string, fields: Partial<Contact>): Promise<Contact> {
  return apiFetch(`/outreach/contacts/${id}`, {
    method: "PATCH",
    body: JSON.stringify(fields),
  });
}

// --- Conversations ----------------------------------------------------

export function listConversations(
  unread = false,
  page = 1,
  pageSize = 25,
): Promise<{
  conversations: ConversationListRow[];
  total: number;
  page: number;
  page_size: number;
}> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (unread) params.set("unread", "true");
  return apiFetch(`/outreach/conversations?${params}`);
}

export function getConversation(id: string): Promise<ConversationDetail> {
  return apiFetch(`/outreach/conversations/${id}`);
}

export function startConversation(
  contact_id: string,
  draft_with_ai = true,
): Promise<StartConversationResult> {
  return apiFetch("/outreach/conversations", {
    method: "POST",
    body: JSON.stringify({ contact_id, draft_with_ai }),
  });
}

export function markConversationRead(id: string): Promise<void> {
  return apiFetch(`/outreach/conversations/${id}/read`, { method: "POST" });
}

export function deleteConversation(id: string): Promise<void> {
  return apiFetch(`/outreach/conversations/${id}`, { method: "DELETE" });
}

export function updateConversation(
  id: string,
  fields: { status?: string; automation?: string },
): Promise<void> {
  return apiFetch(`/outreach/conversations/${id}`, {
    method: "PATCH",
    body: JSON.stringify(fields),
  });
}

export function sendMessage(
  conversationId: string,
  payload: { draft_message_id?: string; subject?: string; body?: string },
): Promise<Message> {
  return apiFetch(`/outreach/conversations/${conversationId}/messages`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function draftReply(conversationId: string): Promise<Message> {
  return apiFetch(`/outreach/conversations/${conversationId}/draft`, { method: "POST" });
}
