"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  getConversation,
  listSenderProfiles,
  listConversationTags,
  sendMessage,
  draftReply,
  redraftReply,
  markConversationRead,
  updateConversation,
} from "@/lib/api/outreach";
import { tagLabel } from "@/components/outreach/intent-tags";
import { useOutreachEvents } from "@/lib/sse/use-outreach-events";
import type { Message, SenderProfile } from "@/lib/types/outreach";
import { AutomationToggle, type AutomationLevel } from "@/components/outreach/automation-toggle";
import { ArrowLeft, Loader2, Paperclip, Send, Sparkles } from "lucide-react";

// ConversationThread is the full-height thread + compose view, reused by both
// the standalone inbox route and the Email Groups nested route. The brand a
// message sends as (and which catalog attaches) is resolved server-side from
// the conversation's campaign; the catalog checkbox here is a hint.
export function ConversationThread({
  conversationId,
  backHref,
  embedded = false,
}: {
  conversationId: string;
  backHref?: string;
  embedded?: boolean;
}) {
  const { data, isLoading, mutate } = useSWR(
    `/outreach/conversations/${conversationId}`,
    () => getConversation(conversationId),
    { refreshInterval: 60000 },
  );

  // Push: revalidate this conversation when an inbound for it arrives.
  useOutreachEvents((e) => {
    if (e.kind === "inbound" && e.conversation_id === conversationId) {
      mutate();
    }
  });

  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [draftMsgID, setDraftMsgID] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [attachCatalog, setAttachCatalog] = useState(true);
  const [adjustText, setAdjustText] = useState("");
  const [remember, setRemember] = useState(false);
  const [adjusting, setAdjusting] = useState(false);

  const { data: profiles } = useSWR<SenderProfile[]>(
    "/outreach/sender-profile",
    () => listSenderProfiles(),
  );
  // A catalog will attach if the conversation's brand has one (resolved
  // server-side). Show the toggle if any brand has a catalog uploaded.
  const hasCatalog = Boolean(
    profiles?.some((p) => p.catalog_file_name && (p.catalog_size_bytes ?? 0) > 0),
  );

  // What a remembered lesson binds to: the conversation's current intent tags
  // (empty = it applies to untagged replies). Only fetched while a draft is
  // loaded, since that's the only time the Remember checkbox is visible.
  const { data: tagData } = useSWR(
    draftMsgID ? `/outreach/conversations/${conversationId}/tags` : null,
    () => listConversationTags(conversationId),
  );

  // On load: mark read + pre-fill compose from any pending draft.
  useEffect(() => {
    if (!data) return;
    if (data.conversation.unread) {
      markConversationRead(conversationId).then(() => mutate());
    }
    const pending = data.messages.find((m) => m.status === "pending_approval");
    if (pending && !draftMsgID) {
      setDraftMsgID(pending.id);
      setSubject(pending.subject ?? "");
      setBody(pending.body_text ?? "");
    } else if (data.conversation.subject && !subject) {
      const base = data.conversation.subject;
      setSubject(base.toLowerCase().startsWith("re:") ? base : `Re: ${base}`);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading conversation…
      </div>
    );
  }
  if (!data) return null;

  async function handleSend() {
    if (!subject.trim() || !body.trim()) {
      setError("Subject and body are required.");
      return;
    }
    setSending(true);
    setError(null);
    try {
      await sendMessage(conversationId, {
        draft_message_id: draftMsgID ?? undefined,
        subject: subject.trim(),
        body: body.trim(),
        attach_catalog: hasCatalog && attachCatalog,
      });
      setBody("");
      setDraftMsgID(null);
      mutate();
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to send";
      // The pre-filled draft is stale (already sent/cancelled elsewhere) —
      // drop it and refetch so the compose box heals.
      if (msg.includes("not a pending draft")) {
        setDraftMsgID(null);
        mutate();
      }
      setError(msg);
    } finally {
      setSending(false);
    }
  }

  async function handleAIDraft() {
    setDrafting(true);
    setError(null);
    try {
      const msg = await draftReply(conversationId);
      setDraftMsgID(msg.id);
      setSubject(msg.subject ?? subject);
      setBody(msg.body_text ?? "");
      mutate();
    } catch (e) {
      setError(e instanceof Error ? e.message : "AI draft failed");
    } finally {
      setDrafting(false);
    }
  }

  async function handleAdjust() {
    if (!draftMsgID || !adjustText.trim()) return;
    setAdjusting(true);
    setError(null);
    try {
      const msg = await redraftReply(conversationId, {
        draft_id: draftMsgID,
        instruction: adjustText.trim(),
        // Send what the user actually sees so the AI revises their manual
        // edits, not the stale stored draft.
        previous_body: body,
        remember,
      });
      setBody(msg.body_text ?? "");
      // Refining a cold opener can rewrite the subject line too.
      if (msg.subject) setSubject(msg.subject);
      setAdjustText("");
      setRemember(false);
      mutate();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Adjust failed");
    } finally {
      setAdjusting(false);
    }
  }

  const msgs = data.messages.filter((m) => m.status !== "pending_approval");

  return (
    <div className={"flex flex-col " + (embedded ? "max-h-[600px]" : "h-[calc(100vh-180px)]")}>
      <div className="flex items-center gap-3 border-b border-border pb-3 mb-3">
        {backHref && (
          <Link href={backHref}>
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
        )}
        <div className="flex-1 min-w-0">
          {!embedded && (
            <h1 className="text-lg font-semibold truncate">
              {data.conversation.subject ?? "No subject"}
            </h1>
          )}
          <p className="text-xs text-muted-foreground font-mono">
            {data.conversation.channel_type} · {data.messages.length} message
            {data.messages.length !== 1 ? "s" : ""}
          </p>
        </div>
        <AutomationToggle
          value={data.conversation.automation as AutomationLevel}
          onChange={async (next) => {
            try {
              await updateConversation(conversationId, { automation: next });
              mutate();
            } catch (e) {
              setError(e instanceof Error ? e.message : "Update failed");
            }
          }}
        />
      </div>

      {error && (
        <Alert variant="destructive" className="mb-3">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="flex-1 overflow-y-auto space-y-3 pb-4">
        {msgs.length === 0 ? (
          <div className="text-sm text-muted-foreground text-center py-10">
            No messages yet — draft one below.
          </div>
        ) : (
          msgs.map((m) => <MessageBubble key={m.id} msg={m} />)
        )}
      </div>

      <Card className="mt-3">
        <CardContent className="pt-4 space-y-3">
          <div className="flex gap-2">
            <Input
              placeholder="Subject"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
            />
            <Button
              variant="outline"
              onClick={handleAIDraft}
              disabled={drafting}
              title="Ask AI to draft a reply"
            >
              {drafting ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <>
                  <Sparkles className="h-4 w-4 mr-1" /> AI draft
                </>
              )}
            </Button>
          </div>
          <Textarea
            placeholder="Type your message…"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={8}
          />
          {draftMsgID && (
            <div className="space-y-1.5 rounded-md border bg-muted/30 p-2">
              <div className="flex gap-2">
                <Input
                  placeholder="Tell the AI what to change — e.g. 'also mention our lead time', 'drop the last line'…"
                  value={adjustText}
                  onChange={(e) => setAdjustText(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      handleAdjust();
                    }
                  }}
                  disabled={adjusting}
                  className="text-sm"
                />
                <Button
                  variant="outline"
                  onClick={handleAdjust}
                  disabled={adjusting || !adjustText.trim()}
                  title="Re-draft incorporating your instruction"
                >
                  {adjusting ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <>
                      <Sparkles className="h-4 w-4 mr-1" /> Adjust
                    </>
                  )}
                </Button>
              </div>
              <label className="flex items-center gap-2 text-xs text-muted-foreground cursor-pointer select-none">
                <input
                  type="checkbox"
                  checked={remember}
                  onChange={(e) => setRemember(e.target.checked)}
                  className="h-3.5 w-3.5 accent-blue-600"
                />
                Remember this for this brand — apply it to future drafts for the same kind of reply
              </label>
              {tagData && (
                <p className="pl-[22px] text-[11px] text-muted-foreground">
                  applies to future replies like this one:{" "}
                  {tagData.tags.length > 0
                    ? tagData.tags.map((t) => tagLabel(t.tag)).join(", ")
                    : "untagged"}
                </p>
              )}
            </div>
          )}
          <div className="flex justify-between items-center gap-3">
            <div className="flex flex-col gap-1 min-w-0">
              {hasCatalog && (
                <label className="flex items-center gap-2 text-xs text-muted-foreground cursor-pointer select-none">
                  <input
                    type="checkbox"
                    checked={attachCatalog}
                    onChange={(e) => setAttachCatalog(e.target.checked)}
                    className="h-3.5 w-3.5 accent-blue-600"
                  />
                  <Paperclip className="h-3.5 w-3.5" />
                  <span className="truncate">Attach file</span>
                </label>
              )}
              {draftMsgID && (
                <p className="text-xs text-muted-foreground">
                  Editing an AI draft — your edits will be sent.
                </p>
              )}
            </div>
            <Button onClick={handleSend} disabled={sending}>
              {sending ? (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              ) : (
                <Send className="h-4 w-4 mr-2" />
              )}
              Send
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function MessageBubble({ msg }: { msg: Message }) {
  const mine = msg.direction === "out";
  const when = msg.sent_at || msg.received_at || msg.created_at;
  const isDraft = msg.status === "draft" || msg.status === "pending_approval";

  let bubbleClass = "max-w-[85%] rounded-lg px-3 py-2 text-sm ";
  let metaClass = "text-[10px] mt-1 ";
  let subjectClass = "text-[11px] mb-1 ";
  if (!mine) {
    bubbleClass += "bg-muted text-foreground";
    metaClass += "text-muted-foreground";
    subjectClass += "text-muted-foreground";
  } else if (isDraft) {
    bubbleClass += "bg-amber-50 text-amber-950 border-2 border-dashed border-amber-300";
    metaClass += "text-amber-700";
    subjectClass += "text-amber-800";
  } else {
    bubbleClass += "bg-blue-600 text-white";
    metaClass += "text-blue-100";
    subjectClass += "text-blue-100";
  }

  return (
    <div className={"flex " + (mine ? "justify-end" : "justify-start")}>
      <div className={bubbleClass}>
        {isDraft && (
          <div className="flex items-center gap-1.5 mb-1.5 pb-1 border-b border-amber-300/60">
            <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-bold tracking-wide bg-amber-200 text-amber-900">
              DRAFT — NOT SENT
            </span>
            {msg.ai_generated && (
              <span className="text-[10px] text-amber-700">AI-generated</span>
            )}
          </div>
        )}
        {msg.subject && <div className={subjectClass}>Subject: {msg.subject}</div>}
        <div className="whitespace-pre-wrap">{msg.body_text ?? stripHTML(msg.body_html ?? "")}</div>
        <div className={metaClass}>
          {isDraft && "Drafted "}
          {new Date(when).toLocaleString("en-GB")}
          {msg.ai_generated && !isDraft && " · AI"}
        </div>
      </div>
    </div>
  );
}

function stripHTML(s: string): string {
  return s.replace(/<[^>]+>/g, "").replace(/\s+/g, " ").trim();
}
