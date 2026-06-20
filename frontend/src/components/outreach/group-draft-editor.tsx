"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { getConversation, updateGroupDraft, approveGroupContact, redraftReply } from "@/lib/api/outreach";
import { Loader2, CheckCircle2, Sparkles } from "lucide-react";

// Inline editor for a not-yet-sent group email draft. Edits are persisted to
// the draft (Save), so the approve → start-sending flow sends the edited copy.
// Save & approve does both in one go.
export function GroupDraftEditor({
  groupId,
  contactId,
  conversationId,
  onChanged,
}: {
  groupId: string;
  contactId: string;
  conversationId: string;
  onChanged: () => void;
}) {
  const { data, mutate } = useSWR(`/outreach/conversations/${conversationId}`, () =>
    getConversation(conversationId),
  );
  const draft = data?.messages.find(
    (m) => m.direction === "out" && (m.status === "draft" || m.status === "pending_approval"),
  );

  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [adjustText, setAdjustText] = useState("");

  useEffect(() => {
    if (draft) {
      setSubject(draft.subject ?? "");
      setBody(draft.body_text ?? "");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draft?.id]);

  if (!data) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground text-sm py-2">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading draft…
      </div>
    );
  }
  if (!draft) {
    return <p className="text-sm text-muted-foreground py-2">No draft yet — still being written.</p>;
  }

  async function save(thenApprove: boolean) {
    if (!subject.trim() || !body.trim()) {
      setMsg("Subject and body are required.");
      return;
    }
    setBusy(thenApprove ? "save-approve" : "save");
    setMsg(null);
    try {
      await updateGroupDraft(groupId, contactId, { subject: subject.trim(), body: body.trim() });
      if (thenApprove) await approveGroupContact(groupId, contactId);
      await mutate();
      onChanged();
      setMsg(thenApprove ? "Saved & approved" : "Saved");
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Save failed");
    } finally {
      setBusy(null);
    }
  }

  async function handleAdjust() {
    if (!draft || !adjustText.trim()) return;
    setBusy("adjust");
    setMsg(null);
    try {
      const updated = await redraftReply(conversationId, {
        draft_id: draft.id,
        instruction: adjustText.trim(),
        previous_body: body,
      });
      if (updated.subject) setSubject(updated.subject);
      setBody(updated.body_text ?? "");
      setAdjustText("");
      await mutate();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Adjust failed");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="inline-block px-1.5 py-0.5 rounded text-[10px] font-bold tracking-wide bg-amber-200 text-amber-900">
          DRAFT — NOT SENT
        </span>
        <span className="text-[10px] text-amber-700">AI-generated · editable</span>
      </div>
      {msg && (
        <Alert>
          <AlertDescription>{msg}</AlertDescription>
        </Alert>
      )}
      <div className="space-y-1">
        <label className="text-xs text-muted-foreground">Subject</label>
        <Input value={subject} onChange={(e) => setSubject(e.target.value)} disabled={busy !== null} />
      </div>
      <div className="space-y-1">
        <label className="text-xs text-muted-foreground">Body</label>
        <Textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          rows={12}
          disabled={busy !== null}
          className="text-sm"
        />
      </div>
      <div className="flex gap-2 rounded-md border bg-muted/30 p-2">
        <Input
          placeholder="Tell the AI what to change — e.g. 'shorter', 'lead with the lead time', 'change the subject'…"
          value={adjustText}
          onChange={(e) => setAdjustText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              handleAdjust();
            }
          }}
          disabled={busy !== null}
          className="text-sm"
        />
        <Button
          size="sm"
          variant="outline"
          onClick={handleAdjust}
          disabled={busy !== null || !adjustText.trim()}
          title="Re-draft incorporating your instruction (can rewrite the subject too)"
        >
          {busy === "adjust" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <>
              <Sparkles className="h-4 w-4 mr-1" /> Adjust
            </>
          )}
        </Button>
      </div>
      <div className="flex justify-end gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={() => save(false)}
          disabled={busy !== null || !subject.trim() || !body.trim()}
        >
          {busy === "save" ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : null}
          Save
        </Button>
        <Button
          size="sm"
          onClick={() => save(true)}
          disabled={busy !== null || !subject.trim() || !body.trim()}
        >
          {busy === "save-approve" ? (
            <Loader2 className="h-4 w-4 animate-spin mr-1" />
          ) : (
            <CheckCircle2 className="h-4 w-4 mr-1" />
          )}
          Save &amp; approve
        </Button>
      </div>
    </div>
  );
}
