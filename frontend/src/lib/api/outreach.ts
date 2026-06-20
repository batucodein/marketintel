import { apiFetch } from "./client";
import type {
  Contact,
  UserChannel,
  SenderProfile,
  ConversationDetail,
  StartConversationResult,
  Message,
  SequenceStep,
  Note,
  BulkEnsureResult,
  ContactGroup,
  GroupSummary,
  GroupDetail,
  GroupEmail,
  CadenceStats,
  EmailFacets,
  ConvTag,
  PlaybookEntry,
  BrandLesson,
  AssistantMessage,
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

// EmailSettings is the autodiscovered IMAP/SMTP config used to prefill the
// connect-other-email form. oauth_hint steers the user to OAuth when set.
export interface EmailSettings {
  imap_host: string;
  imap_port: number;
  smtp_host: string;
  smtp_port: number;
  security: string;
  provider_hint?: string; // "gmail" | "outlook" | "" — shows provider-specific guidance
}

export function detectEmailSettings(email: string): Promise<EmailSettings> {
  return apiFetch("/outreach/channels/email/detect", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export function connectEmail(body: {
  email: string;
  password: string;
  username?: string;
  imap_host?: string;
  imap_port?: number;
  smtp_host?: string;
  smtp_port?: number;
  security?: string;
  display_label?: string;
}): Promise<{ id: string; reconnected?: boolean }> {
  return apiFetch("/outreach/channels/email", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

// --- Sender profiles (brands) -----------------------------------------
// Multi-brand: a user can have several sender profiles, one per market.

export function listSenderProfiles(): Promise<SenderProfile[]> {
  return apiFetch("/outreach/sender-profile");
}

export function getSenderProfile(id: string): Promise<SenderProfile> {
  return apiFetch(`/outreach/sender-profile/${id}`);
}

export function createSenderProfile(p: Partial<SenderProfile>): Promise<SenderProfile> {
  return apiFetch("/outreach/sender-profile", {
    method: "POST",
    body: JSON.stringify(p),
  });
}

export function updateSenderProfile(id: string, p: Partial<SenderProfile>): Promise<SenderProfile> {
  return apiFetch(`/outreach/sender-profile/${id}`, {
    method: "PUT",
    body: JSON.stringify(p),
  });
}

export function deleteSenderProfile(id: string): Promise<void> {
  return apiFetch(`/outreach/sender-profile/${id}`, { method: "DELETE" });
}

export function uploadSenderCatalog(id: string, file: File): Promise<SenderProfile> {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch(`/outreach/sender-profile/${id}/catalog`, {
    method: "POST",
    body: fd,
  });
}

export function deleteSenderCatalog(id: string): Promise<void> {
  return apiFetch(`/outreach/sender-profile/${id}/catalog`, { method: "DELETE" });
}

// --- Contacts ---------------------------------------------------------

export function listContacts(
  opts?: { stage?: string; market_id?: string; page?: number; pageSize?: number },
): Promise<{ contacts: Contact[]; total: number; page: number; page_size: number }> {
  const page = opts?.page ?? 1;
  const pageSize = opts?.pageSize ?? 50;
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (opts?.stage) params.set("stage", opts.stage);
  if (opts?.market_id) params.set("market_id", opts.market_id);
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

// bulkEnsureContacts takes a list of business IDs and creates contacts for
// each, bucketing the result so the UI can show "X added / Y already
// existed / Z had no email". Idempotent — running again after the user
// fills in missing emails promotes those leads from no_email → added.
export function bulkEnsureContacts(business_ids: string[]): Promise<BulkEnsureResult> {
  return apiFetch("/outreach/contacts/ensure-bulk", {
    method: "POST",
    body: JSON.stringify({ business_ids }),
  });
}

// --- Conversations ----------------------------------------------------

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

// redraftReply rewrites the current pending draft per the user's instruction.
// previous_body is the compose box's CURRENT text (including the user's manual
// edits) — the server prefers it over the stored draft row. remember=true saves
// the instruction as a per-brand lesson for this reply type.
export function redraftReply(
  conversationId: string,
  body: { draft_id: string; instruction: string; previous_body?: string; remember?: boolean },
): Promise<Message> {
  return apiFetch(`/outreach/conversations/${conversationId}/draft/refine`, {
    method: "POST",
    body: JSON.stringify(body),
  });
}

// --- Reply playbook (per group) + learned lessons (per brand) ---------

export function getGroupPlaybook(id: string): Promise<{ playbook: PlaybookEntry[] }> {
  return apiFetch(`/outreach/groups/${id}/playbook`);
}

export function saveGroupPlaybook(
  id: string,
  playbook: PlaybookEntry[],
): Promise<{ playbook: PlaybookEntry[] }> {
  return apiFetch(`/outreach/groups/${id}/playbook`, {
    method: "PUT",
    body: JSON.stringify({ playbook }),
  });
}

// --- Draft assistant chat (per group) ----------------------------------

export function getGroupAssistant(id: string): Promise<{ messages: AssistantMessage[] }> {
  return apiFetch(`/outreach/groups/${id}/assistant`);
}

export function sendAssistantMessage(id: string, message: string): Promise<AssistantMessage> {
  return apiFetch(`/outreach/groups/${id}/assistant/messages`, {
    method: "POST",
    body: JSON.stringify({ message }),
  });
}

export function confirmAssistantMessage(
  id: string,
  messageId: string,
): Promise<{ updated: number; failed: number }> {
  return apiFetch(`/outreach/groups/${id}/assistant/messages/${messageId}/confirm`, {
    method: "POST",
  });
}

export function dismissAssistantMessage(id: string, messageId: string): Promise<void> {
  return apiFetch(`/outreach/groups/${id}/assistant/messages/${messageId}/dismiss`, {
    method: "POST",
  });
}

export function listBrandLessons(brandId: string): Promise<{ lessons: BrandLesson[] }> {
  return apiFetch(`/outreach/sender-profile/${brandId}/lessons`);
}

export function deleteBrandLesson(brandId: string, lessonId: string): Promise<void> {
  return apiFetch(`/outreach/sender-profile/${brandId}/lessons/${lessonId}`, { method: "DELETE" });
}

// --- Email Groups ----------------------------------------------------
// A group is a campaign + attached follow-up steps, presented as one thing.

export function listGroups(): Promise<{ groups: GroupSummary[] }> {
  return apiFetch("/outreach/groups");
}

export function getGroup(id: string): Promise<GroupDetail> {
  return apiFetch(`/outreach/groups/${id}`);
}

export function createGroup(body: {
  name: string;
  goal?: string;
  contact_group_id: string;
  send_pace_per_day?: number;
  attach_catalog?: boolean;
  on_positive_action?: string;
  on_negative_action?: string;
  steps?: SequenceStep[];
  channel_id?: string;
}): Promise<GroupSummary> {
  return apiFetch("/outreach/groups", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

// Set the group's reply-branch actions (positive/negative). Backed by the
// campaign PATCH endpoint (same id).
export function updateGroupBranches(
  id: string,
  body: { on_positive_action?: string; on_negative_action?: string },
): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

// --- Contact groups ---------------------------------------------------

export function listContactGroups(): Promise<{ groups: ContactGroup[] }> {
  return apiFetch("/outreach/contact-groups");
}

export function createContactGroup(body: {
  name: string;
  sender_profile_id?: string | null;
  business_ids?: string[];
}): Promise<{ id: string; result: BulkEnsureResult & { added_to_group: number } }> {
  return apiFetch("/outreach/contact-groups", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function setContactGroupBrand(id: string, senderProfileId: string | null): Promise<void> {
  return apiFetch(`/outreach/contact-groups/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ sender_profile_id: senderProfileId }),
  });
}

export function deleteContactGroup(id: string): Promise<void> {
  return apiFetch(`/outreach/contact-groups/${id}`, { method: "DELETE" });
}

export function listContactGroupContacts(id: string): Promise<{ contacts: Contact[]; total: number }> {
  return apiFetch(`/outreach/contact-groups/${id}/contacts`);
}

export function addBusinessesToContactGroup(
  id: string,
  business_ids: string[],
): Promise<BulkEnsureResult & { added_to_group: number }> {
  return apiFetch(`/outreach/contact-groups/${id}/businesses`, {
    method: "POST",
    body: JSON.stringify({ business_ids }),
  });
}

export function patchGroupSteps(id: string, steps: SequenceStep[]): Promise<{ steps: SequenceStep[] }> {
  return apiFetch(`/outreach/groups/${id}/steps`, {
    method: "PUT",
    body: JSON.stringify({ steps }),
  });
}

export function getGroupEmails(
  id: string,
  facets?: EmailFacets,
): Promise<{ emails: GroupEmail[] }> {
  const p = new URLSearchParams();
  facets?.sentiment.forEach((s) => p.append("sentiment", s));
  facets?.tags.forEach((t) => p.append("tag", t));
  facets?.status.forEach((s) => p.append("status", s));
  const q = p.toString() ? `?${p.toString()}` : "";
  return apiFetch(`/outreach/groups/${id}/emails${q}`);
}

export function getGroupCadence(id: string): Promise<CadenceStats> {
  return apiFetch(`/outreach/groups/${id}/cadence`);
}

export function setGroupChannel(id: string, channelId: string): Promise<void> {
  return apiFetch(`/outreach/groups/${id}/channel`, {
    method: "PATCH",
    body: JSON.stringify({ channel_id: channelId }),
  });
}

// Manual intent-tag editing on a conversation. The classifier never clobbers a
// manual tag; manual remove deletes regardless of source.
export function listConversationTags(convId: string): Promise<{ tags: ConvTag[] }> {
  return apiFetch(`/outreach/conversations/${convId}/tags`);
}

export function addConversationTag(convId: string, tag: string): Promise<{ tags: ConvTag[] }> {
  return apiFetch(`/outreach/conversations/${convId}/tags`, {
    method: "POST",
    body: JSON.stringify({ tag }),
  });
}

export function removeConversationTag(convId: string, tag: string): Promise<{ tags: ConvTag[] }> {
  return apiFetch(`/outreach/conversations/${convId}/tags/${encodeURIComponent(tag)}`, {
    method: "DELETE",
  });
}

// Group lifecycle actions. A group is backed by a campaign with the same id,
// so these hit the campaign endpoints that are still mounted.
export function deleteGroup(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}`, { method: "DELETE" });
}

// Toggle whether the brand's attachment rides along on every email in the group.
export function updateGroupAttachment(id: string, attach: boolean): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ attach_catalog: attach }),
  });
}

export function approveGroupContact(id: string, contactId: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/contacts/${contactId}/approve`, { method: "POST" });
}

// Edit a group email's draft (subject + body) before approval. Backed by the
// campaign draft-edit endpoint; only works while the row is still "drafted".
export function updateGroupDraft(
  id: string,
  contactId: string,
  payload: { subject: string; body: string },
): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/contacts/${contactId}/draft`, {
    method: "PATCH",
    body: JSON.stringify(payload),
  });
}

export function approveAllGroup(id: string): Promise<{ approved: number }> {
  return apiFetch(`/outreach/campaigns/${id}/approve-all`, { method: "POST" });
}

export function launchGroup(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/launch`, { method: "POST" });
}

export function pauseGroup(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/pause`, { method: "POST" });
}

export function resumeGroup(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/resume`, { method: "POST" });
}

export function stopGroup(id: string): Promise<void> {
  return apiFetch(`/outreach/campaigns/${id}/stop`, { method: "POST" });
}

// --- Notes ------------------------------------------------------------

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
