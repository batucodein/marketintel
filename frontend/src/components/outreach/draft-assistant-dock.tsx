"use client";

import { useState, useRef, useEffect } from "react";
import useSWR from "swr";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  getGroupAssistant,
  sendAssistantMessage,
  confirmAssistantMessage,
  dismissAssistantMessage,
  getGroupPlaybook,
} from "@/lib/api/outreach";
import type { AssistantMessage, PlaybookEntry } from "@/lib/types/outreach";
import { tagLabel } from "@/components/outreach/intent-tags";
import { useOutreachEvents } from "@/lib/sse/use-outreach-events";
import { Sparkles, ChevronDown, Loader2, Send, Brain } from "lucide-react";

// toolStatus maps an agent tool name to a short, human "thinking" line shown
// transiently while the agent gathers what it needs to answer.
function toolStatus(tool: string): string {
  switch (tool) {
    case "counts":
      return "counting drafts & replies…";
    case "list_contacts":
    case "list_leads":
      return "looking up the companies…";
    case "list_drafts":
      return "scanning drafts…";
    case "get_draft":
      return "reading a draft…";
    case "list_replies":
      return "scanning replies…";
    case "get_reply":
      return "reading a reply…";
    case "get_lead":
      return "checking lead score…";
    case "top_leads":
      return "ranking leads…";
    case "get_brand":
      return "checking the brand…";
    case "get_playbook":
      return "checking the playbook…";
    case "get_campaign_setup":
      return "checking the setup…";
    default:
      return "thinking…";
  }
}

// AssistantSource abstracts where the dock reads/writes — a live group or a
// simulation. The agent loop, prompt, and tools are shared on the backend; this
// is the thin data binding so one dock serves both.
export interface AssistantSource {
  id: string;
  swrKey: string;
  eventKind: string; // "campaign_progress" | "simulation_progress"
  playbook: PlaybookEntry[];
  loadHistory: () => Promise<{ messages: AssistantMessage[] }>;
  send: (text: string) => Promise<AssistantMessage>;
  confirm: (mid: string) => Promise<unknown>;
  dismiss: (mid: string) => Promise<void>;
}

// AssistantDock is the bottom-right chat agent. It answers questions about the
// drafts/replies/leads and proposes confirm-gated write actions (edit a draft
// cohort, add/remove a playbook rule). Its only memory is the playbook (shown
// read-only in Memory).
export function AssistantDock({
  source,
  onChanged,
}: {
  source: AssistantSource;
  onChanged?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [showMemory, setShowMemory] = useState(false);
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [confirming, setConfirming] = useState<string | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [thinking, setThinking] = useState<string | null>(null);
  const [pendingUser, setPendingUser] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  const { data, mutate } = useSWR(source.swrKey, () => source.loadHistory());
  const messages = data?.messages ?? [];
  const playbook = source.playbook;

  useOutreachEvents((e) => {
    if (e.kind !== source.eventKind) return;
    if (e.campaign_id !== source.id && e.data?.simulation_id !== source.id) return;
    if (e.data?.action === "assistant_thinking") {
      setThinking(toolStatus(String(e.data.tool ?? "")));
    } else if (e.data?.action === "directive" || e.data?.action === "assistant_apply") {
      setProgress({ done: Number(e.data.done ?? 0), total: Number(e.data.total ?? 0) });
    }
  });

  useEffect(() => {
    if (open && scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [open, messages.length, pendingUser, sending]);

  async function send() {
    const t = text.trim();
    if (!t || sending) return;
    setSending(true);
    setPendingUser(t);
    setThinking(null);
    setText("");
    try {
      await source.send(t);
      await mutate();
    } catch {
      setText(t);
    } finally {
      setPendingUser(null);
      setSending(false);
      setThinking(null);
    }
  }

  async function confirm(messageId: string) {
    setConfirming(messageId);
    setProgress(null);
    try {
      await source.confirm(messageId);
      await mutate();
      onChanged?.();
    } finally {
      setConfirming(null);
      setProgress(null);
    }
  }

  async function dismiss(messageId: string) {
    await source.dismiss(messageId);
    mutate();
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="fixed bottom-4 right-4 z-40 flex items-center gap-2 rounded-full border border-primary/40 bg-background px-4 py-2.5 text-sm font-medium shadow-lg hover:border-primary transition cursor-pointer"
      >
        <Sparkles className="h-4 w-4 text-primary" /> AI draft assistant
      </button>
    );
  }

  return (
    <div className="fixed bottom-4 right-4 z-40 flex w-[min(420px,calc(100vw-2rem))] h-[70vh] max-h-[640px] flex-col rounded-xl border bg-background shadow-2xl">
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <Sparkles className="h-4 w-4 text-primary" />
        <span className="text-sm font-semibold flex-1">AI draft assistant</span>
        <button
          type="button"
          onClick={() => setShowMemory((v) => !v)}
          className={
            "flex items-center gap-1 rounded-md px-2 py-1 text-xs transition cursor-pointer " +
            (showMemory ? "bg-primary/10 text-primary" : "text-muted-foreground hover:bg-muted")
          }
          title="What the AI remembers for replies (the playbook)"
        >
          <Brain className="h-3.5 w-3.5" /> Memory ({playbook.length})
        </button>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="text-muted-foreground hover:text-foreground cursor-pointer"
          title="Collapse"
        >
          <ChevronDown className="h-4 w-4" />
        </button>
      </div>

      {showMemory && (
        <div className="border-b bg-muted/30 px-3 py-2 max-h-40 overflow-y-auto space-y-1">
          <p className="text-[11px] text-muted-foreground">
            The reply playbook — what the AI follows when drafting replies. Ask me to add or remove a
            rule, or edit it in the <span className="font-medium">Reply playbook</span>.
          </p>
          {playbook.length === 0 ? (
            <p className="text-xs text-muted-foreground py-1">Nothing remembered yet.</p>
          ) : (
            playbook.map((p) => (
              <div key={p.tag} className="rounded-md border bg-background px-2 py-1">
                <span className="text-xs">
                  <span className="font-medium">{tagLabel(p.tag)}:</span> {p.instruction}
                </span>
              </div>
            ))
          )}
        </div>
      )}

      <div ref={scrollRef} className="flex-1 overflow-y-auto px-3 py-3 space-y-3">
        {messages.length === 0 && !pendingUser && (
          <p className="text-xs text-muted-foreground text-center py-4">
            Ask about your drafts and replies, or tell me what to change — e.g. &quot;who replied
            positively?&quot;, &quot;make the positive-reply drafts warmer&quot;, or &quot;always
            send the price list when they ask price&quot;.
          </p>
        )}
        {messages.map((m: AssistantMessage) => (
          <div key={m.id} className={"flex " + (m.role === "user" ? "justify-end" : "justify-start")}>
            <div
              className={
                "max-w-[85%] rounded-lg px-3 py-2 text-sm whitespace-pre-wrap " +
                (m.role === "user" ? "bg-primary text-primary-foreground" : "bg-muted text-foreground")
              }
            >
              {m.content}
              {m.role === "assistant" && m.status === "proposed" && m.proposed_action && (
                <div className="mt-2 flex flex-col gap-1.5">
                  {confirming === m.id ? (
                    <span className="text-xs text-muted-foreground flex items-center gap-1.5">
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      {progress ? `Updating drafts ${progress.done}/${progress.total}…` : "Working…"}
                    </span>
                  ) : (
                    <div className="flex gap-2">
                      <Button size="sm" onClick={() => confirm(m.id)}>
                        <Sparkles className="h-3.5 w-3.5 mr-1" />
                        {m.proposed_action.type === "edit_drafts" ? "Confirm & apply" : "Confirm"}
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => dismiss(m.id)}>
                        Dismiss
                      </Button>
                    </div>
                  )}
                </div>
              )}
              {m.role === "assistant" && m.status === "dismissed" && (
                <div className="mt-1 text-[10px] text-muted-foreground">Dismissed</div>
              )}
            </div>
          </div>
        ))}
        {pendingUser && (
          <div className="flex justify-end">
            <div className="max-w-[85%] rounded-lg px-3 py-2 text-sm bg-primary text-primary-foreground whitespace-pre-wrap opacity-70">
              {pendingUser}
            </div>
          </div>
        )}
        {sending && (
          <div className="flex justify-start">
            <div className="rounded-lg px-3 py-2 bg-muted text-muted-foreground flex items-center gap-2 text-xs">
              <Loader2 className="h-4 w-4 animate-spin shrink-0" />
              {thinking && <span>{thinking}</span>}
            </div>
          </div>
        )}
      </div>

      <div className="border-t p-2 flex gap-2">
        <Textarea
          placeholder="Message the assistant…"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              send();
            }
          }}
          rows={1}
          disabled={sending}
          className="text-sm min-h-9 resize-none"
        />
        <Button size="icon" onClick={send} disabled={sending || !text.trim()} title="Send">
          <Send className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

// DraftAssistantDock is the group binding — kept so existing group call sites
// work unchanged. It builds a group AssistantSource and renders AssistantDock.
export function DraftAssistantDock({
  groupId,
  onDraftsChanged,
}: {
  groupId: string;
  onDraftsChanged: () => void;
}) {
  const { data: pbData, mutate: mutatePlaybook } = useSWR(
    `/outreach/groups/${groupId}/playbook`,
    () => getGroupPlaybook(groupId),
  );
  const source: AssistantSource = {
    id: groupId,
    swrKey: `/outreach/groups/${groupId}/assistant`,
    eventKind: "campaign_progress",
    playbook: pbData?.playbook ?? [],
    loadHistory: () => getGroupAssistant(groupId),
    send: (t) => sendAssistantMessage(groupId, t),
    confirm: (mid) => confirmAssistantMessage(groupId, mid),
    dismiss: (mid) => dismissAssistantMessage(groupId, mid),
  };
  return (
    <AssistantDock
      source={source}
      onChanged={() => {
        mutatePlaybook();
        onDraftsChanged();
      }}
    />
  );
}
