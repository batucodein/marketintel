import { apiFetch } from "./client";
import type {
  Contact,
  UserChannel,
  SenderProfile,
  ConversationListRow,
  ConversationDetail,
  StartConversationResult,
  Message,
  Campaign,
  CampaignSummary,
  CampaignContactRow,
  AddContactsResult,
  Sequence,
  SequenceStep,
  SequenceWithSteps,
  Task,
  Note,
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

export function uploadSenderCatalog(file: File): Promise<SenderProfile> {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch("/outreach/sender-profile/catalog", {
    method: "POST",
    body: fd,
  });
}

export function deleteSenderCatalog(): Promise<void> {
  return apiFetch("/outreach/sender-profile/catalog", { method: "DELETE" });
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
  payload: {
    draft_message_id?: string;
    subject?: string;
    body?: string;
    attach_catalog?: boolean;
  },
): Promise<Message> {
  return apiFetch(`/outreach/conversations/${conversationId}/messages`, {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function draftReply(conversationId: string): Promise<Message> {
  return apiFetch(`/outreach/conversations/${conversationId}/draft`, { method: "POST" });
}

// --- Campaigns -------------------------------------------------------

export function listCampaigns(): Promise<{ campaigns: Campaign[] }> {
  return apiFetch("/outreach/campaigns/");
}

export function getCampaign(id: string): Promise<CampaignSummary> {
  return apiFetch(`/outreach/campaigns/${id}`);
}

export function createCampaign(body: {
  name: string;
  goal?: string;
  channel_id?: string;
  positioning_override?: Record<string, unknown> | null;
  sequence_id?: string | null;
  send_pace_per_day?: number;
  attach_catalog?: boolean;
}): Promise<Campaign> {
  return apiFetch("/outreach/campaigns/", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateCampaign(id: string, body: Partial<Campaign>): Promise<Campaign> {
  return apiFetch(`/outreach/campaigns/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteCampaign(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}`, { method: "DELETE" });
}

export function listCampaignContacts(
  id: string,
  status?: string,
): Promise<{ contacts: CampaignContactRow[] }> {
  const q = status ? `?status=${encodeURIComponent(status)}` : "";
  return apiFetch(`/outreach/campaigns/${id}/contacts${q}`);
}

export function addContactsToCampaign(
  id: string,
  contact_ids: string[],
  opts?: { market_id?: string; force?: boolean },
): Promise<AddContactsResult> {
  return apiFetch(`/outreach/campaigns/${id}/contacts`, {
    method: "POST",
    body: JSON.stringify({ contact_ids, ...opts }),
  });
}

export function approveCampaignContact(id: string, contactId: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/contacts/${contactId}/approve`, { method: "POST" });
}

export function approveAllCampaignContacts(id: string): Promise<{ approved: number }> {
  return apiFetch(`/outreach/campaigns/${id}/approve-all`, { method: "POST" });
}

export function launchCampaign(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/launch`, { method: "POST" });
}

export function pauseCampaign(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/pause`, { method: "POST" });
}

export function resumeCampaign(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/resume`, { method: "POST" });
}

export function stopCampaign(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/stop`, { method: "POST" });
}

// --- Sequences -------------------------------------------------------

export function listSequences(): Promise<{ sequences: Sequence[] }> {
  return apiFetch("/outreach/sequences/");
}

export function getSequence(id: string): Promise<SequenceWithSteps> {
  return apiFetch(`/outreach/sequences/${id}`);
}

export function createSequence(body: {
  name: string;
  description?: string;
  is_template?: boolean;
  steps: SequenceStep[];
}): Promise<SequenceWithSteps> {
  return apiFetch("/outreach/sequences/", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateSequence(id: string, body: { name?: string; description?: string }): Promise<Sequence> {
  return apiFetch(`/outreach/sequences/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function replaceSequenceSteps(id: string, steps: SequenceStep[]): Promise<{ steps: SequenceStep[] }> {
  return apiFetch(`/outreach/sequences/${id}/steps`, {
    method: "POST",
    body: JSON.stringify({ steps }),
  });
}

export function deleteSequence(id: string): Promise<void> {
  return apiFetch(`/outreach/sequences/${id}`, { method: "DELETE" });
}

// --- Contacts: bulk update -------------------------------------------

export function bulkUpdateContacts(body: {
  ids: string[];
  pipeline_stage?: string;
  default_automation?: string;
  default_sequence_id?: string;
}): Promise<{ updated: number }> {
  return apiFetch("/outreach/contacts/bulk", {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

// --- Tasks + notes ----------------------------------------------------

export function listTasks(opts?: { contact_id?: string; openOnly?: boolean }): Promise<{ tasks: Task[] }> {
  const p = new URLSearchParams();
  if (opts?.contact_id) p.set("contact_id", opts.contact_id);
  if (opts?.openOnly) p.set("status", "open");
  const qs = p.toString();
  return apiFetch(`/outreach/tasks${qs ? "?" + qs : ""}`);
}

export function createTask(body: {
  title: string;
  body?: string;
  contact_id?: string;
  conversation_id?: string;
  due_at?: string;
}): Promise<Task> {
  return apiFetch("/outreach/tasks", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function updateTask(id: string, body: {
  title?: string;
  body?: string;
  due_at?: string | null;
  completed_at?: string | null;
}): Promise<Task> {
  return apiFetch(`/outreach/tasks/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export function deleteTask(id: string): Promise<void> {
  return apiFetch(`/outreach/tasks/${id}`, { method: "DELETE" });
}

export function tasksOverdueCount(): Promise<{ overdue: number }> {
  return apiFetch("/outreach/tasks/overdue-count");
}

export function listContactNotes(contactID: string): Promise<{ notes: Note[] }> {
  return apiFetch(`/outreach/contacts/${contactID}/notes`);
}

export function createContactNote(contactID: string, bodyText: string): Promise<Note> {
  return apiFetch(`/outreach/contacts/${contactID}/notes`, {
    method: "POST",
    body: JSON.stringify({ body: bodyText }),
  });
}

export function deleteContactNote(contactID: string, noteID: string): Promise<void> {
  return apiFetch(`/outreach/contacts/${contactID}/notes/${noteID}`, { method: "DELETE" });
}
