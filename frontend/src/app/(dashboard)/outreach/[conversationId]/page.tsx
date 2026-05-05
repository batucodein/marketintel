"use client";

import { use, useEffect, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  getConversation,
  getSenderProfile,
  sendMessage,
  draftReply,
  markConversationRead,
  updateConversation,
} from "@/lib/api/outreach";
import { useOutreachEvents } from "@/lib/sse/use-outreach-events";
import type { Message, SenderProfile } from "@/lib/types/outreach";
import { AutomationToggle, type AutomationLevel } from "@/components/outreach/automation-toggle";
import { ArrowLeft, Loader2, Paperclip, Send, Sparkles } from "lucide-react";

type PageProps = { params: Promise<{ conversationId: string }> };

export default function ConversationPage({ params }: PageProps) {
  const { conversationId } = use(params);
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

  const { data: profile } = useSWR<SenderProfile>(
    "/outreach/sender-profile",
    () => getSenderProfile(),
  );
  const hasCatalog = Boolean(profile?.catalog_file_name && (profile?.catalog_size_bytes ?? 0) > 0);

  // On load: mark the conversation read and pre-fill compose from any pending draft.
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
      // Keep subject for thread continuity
      mutate();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to send");
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

  const msgs = data.messages.filter((m) => m.status !== "pending_approval");

  return (
    <div className="flex flex-col h-[calc(100vh-180px)]">
      <div className="flex items-center gap-3 border-b border-border pb-3 mb-3">
        <Link href="/outreach">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div className="flex-1 min-w-0">
          <h1 className="text-lg font-semibold truncate">
            {data.conversation.subject ?? "No subject"}
          </h1>
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
                  <span className="truncate">
                    Attach catalog ({profile?.catalog_file_name})
                  </span>
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
  return (
    <div className={"flex " + (mine ? "justify-end" : "justify-start")}>
      <div
        className={
          "max-w-[85%] rounded-lg px-3 py-2 text-sm " +
          (mine ? "bg-blue-600 text-white" : "bg-muted text-foreground")
        }
      >
        {msg.subject && <div className={"text-[11px] mb-1 " + (mine ? "text-blue-100" : "text-muted-foreground")}>Subject: {msg.subject}</div>}
        <div className="whitespace-pre-wrap">{msg.body_text ?? stripHTML(msg.body_html ?? "")}</div>
        <div className={"text-[10px] mt-1 " + (mine ? "text-blue-100" : "text-muted-foreground")}>
          {new Date(when).toLocaleString("en-GB")}
          {msg.ai_generated && " · AI draft"}
        </div>
      </div>
    </div>
  );
}

function stripHTML(s: string): string {
  return s.replace(/<[^>]+>/g, "").replace(/\s+/g, " ").trim();
}
