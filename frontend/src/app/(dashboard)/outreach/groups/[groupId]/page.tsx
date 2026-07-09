"use client";

import { use, useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import useSWR, { mutate as globalMutate } from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  getGroup,
  getGroupEmails,
  patchGroupSteps,
  deleteGroup,
  approveGroupContact,
  approveAllGroup,
  launchGroup,
  pauseGroup,
  resumeGroup,
  stopGroup,
  updateGroupAttachment,
  updateGroupBranches,
  listSenderProfiles,
  getGroupPlaybook,
  saveGroupPlaybook,
} from "@/lib/api/outreach";
import type { GroupDetail, GroupEmail, EmailFacets, SequenceStep, SenderProfile, PlaybookEntry } from "@/lib/types/outreach";
import { FollowupStepEditor } from "@/components/outreach/followup-step-editor";
import { PlaybookEditor } from "@/components/outreach/playbook-editor";
import { DraftAssistantDock } from "@/components/outreach/draft-assistant-dock";
import { FacetFilterBar } from "@/components/outreach/facet-filter-bar";
import { CadencePanel } from "@/components/outreach/cadence-panel";
import { SendFromRow } from "@/components/outreach/send-from-row";
import { SentimentBadge } from "@/components/outreach/sentiment-badge";
import { TagChip } from "@/components/outreach/tag-chip";
import { ConversationThread } from "@/components/outreach/conversation-thread";
import { GroupDraftEditor } from "@/components/outreach/group-draft-editor";
import { useOutreachEvents } from "@/lib/sse/use-outreach-events";
import {
  Loader2,
  Bell,
  Trash2,
  Play,
  Pause,
  StopCircle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
} from "lucide-react";

type PageProps = { params: Promise<{ groupId: string }> };

const EMPTY_FACETS: EmailFacets = { sentiment: [], tags: [], status: [] };

const STATUS_LABEL: Record<string, string> = {
  pending: "Drafting…",
  drafted: "Awaiting approval",
  approved: "Scheduled",
  sent: "Sent",
  replied: "Replied",
  cold: "Cold",
  skipped: "Skipped",
  failed: "Failed",
};

export default function GroupDetailPage({ params }: PageProps) {
  const { groupId } = use(params);
  const router = useRouter();
  const [facets, setFacets] = useState<EmailFacets>(EMPTY_FACETS);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: group, mutate: mutateGroup } = useSWR<GroupDetail>(
    `/outreach/groups/${groupId}`,
    () => getGroup(groupId),
    { refreshInterval: 10000 },
  );
  const { data: emailsData, mutate: mutateEmails } = useSWR(
    [`/outreach/groups/${groupId}/emails`, facets],
    () => getGroupEmails(groupId, facets),
    { refreshInterval: 10000 },
  );
  const { data: profiles } = useSWR<SenderProfile[]>("/outreach/sender-profile", () =>
    listSenderProfiles(),
  );

  useOutreachEvents((e) => {
    if (e.kind === "inbound") {
      mutateGroup();
      mutateEmails();
    }
  });

  const [steps, setSteps] = useState<SequenceStep[]>([]);
  const [stepsDirty, setStepsDirty] = useState(false);
  const [savingSteps, setSavingSteps] = useState(false);
  const [stepMsg, setStepMsg] = useState<string | null>(null);
  const [onPos, setOnPos] = useState("auto_draft_reply");
  const [onNeg, setOnNeg] = useState("mark_cold");
  const [playbook, setPlaybook] = useState<PlaybookEntry[]>([]);
  const [playbookDirty, setPlaybookDirty] = useState(false);
  const [savingPlaybook, setSavingPlaybook] = useState(false);
  const [playbookMsg, setPlaybookMsg] = useState<string | null>(null);

  // Don't clobber unsaved local edits with the 10s poll — only sync when clean.
  useEffect(() => {
    if (group?.steps && !stepsDirty) setSteps(group.steps);
  }, [group?.steps, stepsDirty]);

  useEffect(() => {
    if (group?.on_positive_action) setOnPos(group.on_positive_action);
    if (group?.on_negative_action) setOnNeg(group.on_negative_action);
  }, [group?.on_positive_action, group?.on_negative_action]);

  const { data: playbookData } = useSWR(
    `/outreach/groups/${groupId}/playbook`,
    () => getGroupPlaybook(groupId),
  );
  useEffect(() => {
    if (playbookData?.playbook && !playbookDirty) setPlaybook(playbookData.playbook);
  }, [playbookData?.playbook, playbookDirty]);

  async function savePlaybook() {
    setSavingPlaybook(true);
    setPlaybookMsg(null);
    try {
      const res = await saveGroupPlaybook(groupId, playbook);
      setPlaybook(res.playbook);
      setPlaybookDirty(false);
      setPlaybookMsg("Playbook saved");
    } catch (e) {
      setPlaybookMsg(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSavingPlaybook(false);
    }
  }

  async function saveBranches(nextPos: string, nextNeg: string) {
    const prevPos = onPos;
    const prevNeg = onNeg;
    setOnPos(nextPos);
    setOnNeg(nextNeg);
    try {
      await updateGroupBranches(groupId, { on_positive_action: nextPos, on_negative_action: nextNeg });
      mutateGroup();
    } catch (e) {
      // Roll the selects back so they don't show a value that never saved.
      setOnPos(prevPos);
      setOnNeg(prevNeg);
      setError(e instanceof Error ? e.message : "Failed to save reply handling");
    }
  }

  async function refresh() {
    await Promise.all([mutateGroup(), mutateEmails()]);
  }

  async function action(name: string, fn: () => Promise<unknown>) {
    setBusy(name);
    setError(null);
    try {
      await fn();
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : `${name} failed`);
    } finally {
      setBusy(null);
    }
  }

  async function handleDelete() {
    if (
      !confirm(
        "Delete this email group?\n\n" +
          "• Scheduled (unsent) emails are cancelled\n" +
          "• Sent conversations stay in your inbox\n" +
          "• Follow-ups stop\n\nThis cannot be undone.",
      )
    )
      return;
    setBusy("delete");
    setError(null);
    try {
      await deleteGroup(groupId);
      // Revalidate the groups list so the left rail + index drop it immediately.
      await globalMutate("/outreach/groups");
      router.push("/outreach/groups");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Delete failed");
      setBusy(null);
    }
  }

  async function saveSteps() {
    setSavingSteps(true);
    setStepMsg(null);
    try {
      await patchGroupSteps(groupId, steps);
      setStepsDirty(false);
      setStepMsg("Follow-up steps saved");
      mutateGroup();
    } catch (e) {
      setStepMsg(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSavingSteps(false);
    }
  }

  if (!group) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading group…
      </div>
    );
  }

  const emails = emailsData?.emails ?? [];
  const status = group.status;
  const isDraft = status === "draft" || status === "ready";
  const isActive = status === "active";
  const isPaused = status === "paused";
  const hasDrafted = group.drafted_count > 0;

  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-bold truncate">{group.name}</h1>
          <p className="text-sm text-muted-foreground">
            {group.brand_name || "no brand"}
            {group.contact_group_name ? ` · ${group.contact_group_name}` : ""} · {group.total_count} contacts ·{" "}
            <span className="capitalize">{status}</span>
          </p>
          <div className="mt-1">
            <SendFromRow
              groupId={groupId}
              channelId={group.channel_id}
              senderEmail={group.sender_email}
              editable={isDraft}
              onChanged={refresh}
            />
          </div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="text-red-600 hover:text-red-700 hover:bg-red-50 shrink-0"
          onClick={handleDelete}
          disabled={busy !== null}
        >
          {busy === "delete" ? (
            <Loader2 className="h-4 w-4 animate-spin mr-1" />
          ) : (
            <Trash2 className="h-4 w-4 mr-1" />
          )}
          Delete
        </Button>
      </div>

      {/* Lifecycle controls */}
      <div className="flex flex-wrap gap-2">
        {hasDrafted && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => action("approve-all", () => approveAllGroup(groupId))}
            disabled={busy !== null}
          >
            <CheckCircle2 className="h-4 w-4 mr-1" /> Approve all ({group.drafted_count})
          </Button>
        )}
        {isDraft && (
          <Button
            size="sm"
            onClick={() => action("launch", () => launchGroup(groupId))}
            disabled={busy !== null || group.approved_count === 0}
            title={
              group.approved_count === 0
                ? "Approve at least one email first"
                : "Start sending approved emails at your pace, then run follow-ups"
            }
          >
            <Play className="h-4 w-4 mr-1" /> Start sending ({group.approved_count})
          </Button>
        )}
        {isActive && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => action("pause", () => pauseGroup(groupId))}
            disabled={busy !== null}
          >
            <Pause className="h-4 w-4 mr-1" /> Pause
          </Button>
        )}
        {isPaused && (
          <Button
            size="sm"
            onClick={() => action("resume", () => resumeGroup(groupId))}
            disabled={busy !== null}
          >
            <Play className="h-4 w-4 mr-1" /> Resume
          </Button>
        )}
        {(isActive || isPaused) && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              if (
                confirm(
                  "End this group?\n\nThis is permanent — it stops all cold sends and follow-ups and can't be resumed. (Use Pause if you just want to hold it.)",
                )
              ) {
                action("stop", () => stopGroup(groupId));
              }
            }}
            disabled={busy !== null}
          >
            <StopCircle className="h-4 w-4 mr-1" /> End
          </Button>
        )}
      </div>

      {isPaused && (
        <Alert>
          <AlertDescription>
            This group is <span className="font-medium">paused</span> — no cold emails or follow-ups
            are going out. When you Resume, the whole remaining schedule shifts forward by however
            long it was paused (nothing fires retroactively).
          </AlertDescription>
        </Alert>
      )}

      {(() => {
        const brand = profiles?.find((p) => p.id === group.brand_id);
        if (!brand?.catalog_file_name) return null;
        return (
          <label className="flex items-center gap-2 text-sm cursor-pointer w-fit">
            <input
              type="checkbox"
              checked={group.attach_catalog}
              disabled={busy !== null}
              onChange={(e) =>
                action("attachment", () => updateGroupAttachment(groupId, e.target.checked))
              }
              className="h-4 w-4 accent-blue-600"
            />
            Attach <span className="font-medium">{brand.catalog_file_name}</span> to every email
          </label>
        );
      })()}

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_360px] gap-4">
        {/* Emails + filters */}
        <div className="space-y-3">
          <CadencePanel groupId={groupId} />
          <FacetFilterBar
            facets={facets}
            counts={group.facet_counts ?? {}}
            onChange={setFacets}
          />
          {facets.status.includes("no_reply") && facets.sentiment.length > 0 && (
            <p className="text-xs text-muted-foreground">
              No-reply rows have no sentiment — these filters together match nothing.
            </p>
          )}

          {emails.length === 0 ? (
            <p className="text-sm text-muted-foreground py-6 text-center">
              No emails in this filter.
            </p>
          ) : (
            <div className="space-y-2">
              {emails.map((e: GroupEmail) => {
                const isOpen = expanded === e.contact_id;
                return (
                  <Card key={e.contact_id} className="overflow-hidden">
                    <div className="p-3 flex items-center gap-3">
                      <button
                        className="flex items-center gap-2 min-w-0 flex-1 text-left"
                        onClick={() =>
                          e.conversation_id && setExpanded(isOpen ? null : e.contact_id)
                        }
                        disabled={!e.conversation_id}
                      >
                        {e.conversation_id ? (
                          isOpen ? (
                            <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
                          ) : (
                            <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                          )
                        ) : (
                          <span className="w-4 shrink-0" />
                        )}
                        <span className="min-w-0">
                          <span className="font-medium truncate block">{e.contact_name}</span>
                          <span className="text-xs text-muted-foreground truncate block">
                            {e.business_name}
                            {e.contact_email ? ` · ${e.contact_email}` : ""}
                          </span>
                        </span>
                      </button>
                      {e.last_direction === "in" && (
                        <SentimentBadge
                          label={e.sentiment_label}
                          legacy={e.sentiment}
                          score={e.sentiment_score}
                        />
                      )}
                      {e.tags?.slice(0, 3).map((t) => <TagChip key={t} tag={t} />)}
                      {e.tags && e.tags.length > 3 && (
                        <span className="text-[10px] text-muted-foreground shrink-0">
                          +{e.tags.length - 3}
                        </span>
                      )}
                      {e.has_pending_draft && (
                        <Badge
                          variant="outline"
                          className="text-[10px] whitespace-nowrap shrink-0 text-amber-700 border-amber-300 bg-amber-50"
                        >
                          Draft ready
                        </Badge>
                      )}
                      <Badge variant="outline" className="text-[10px] whitespace-nowrap shrink-0">
                        {STATUS_LABEL[e.status] ?? e.status}
                      </Badge>
                      {e.status === "approved" &&
                        e.scheduled_send_at &&
                        (new Date(e.scheduled_send_at) <= new Date() ? (
                          // Past-due but still approved: the scheduler picks it
                          // up on its next cycle (~1 min). A stale "sends <past
                          // time>" here read as a bug, so say what's happening.
                          <span
                            className="text-[10px] text-amber-600 whitespace-nowrap shrink-0"
                            title={`Was due ${new Date(e.scheduled_send_at).toLocaleString("en-GB")} — sends on the next cycle (within a minute). If it stays like this, check the sending account in Settings → Email channels.`}
                          >
                            sending…
                          </span>
                        ) : (
                          <span
                            className="text-[10px] text-muted-foreground whitespace-nowrap shrink-0"
                            title={`Scheduled to send ${new Date(e.scheduled_send_at).toLocaleString("en-GB")}`}
                          >
                            sends{" "}
                            {new Date(e.scheduled_send_at).toLocaleString("en-GB", {
                              day: "2-digit",
                              month: "short",
                              hour: "2-digit",
                              minute: "2-digit",
                            })}
                          </span>
                        ))}
                      {(e.status === "failed" || e.status === "skipped") && e.skip_reason && (
                        <span
                          className="text-[10px] text-red-600 truncate max-w-[280px] shrink"
                          title={e.skip_reason}
                        >
                          {e.skip_reason}
                        </span>
                      )}
                      {e.status === "failed" && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="shrink-0"
                          onClick={() =>
                            action(`approve-${e.contact_id}`, () =>
                              approveGroupContact(groupId, e.contact_id),
                            )
                          }
                          disabled={busy !== null}
                        >
                          Retry
                        </Button>
                      )}
                      {e.next_run_at && e.current_step != null && steps.length > 0 && (
                        <span
                          className="text-[10px] text-muted-foreground whitespace-nowrap shrink-0"
                          title={`Next follow-up fires ${new Date(e.next_run_at).toLocaleString("en-GB")}`}
                        >
                          Step {Math.min(e.current_step, steps.length)}/{steps.length} ·{" "}
                          {new Date(e.next_run_at).toLocaleString("en-GB", {
                            day: "2-digit",
                            month: "short",
                            hour: "2-digit",
                            minute: "2-digit",
                          })}
                        </span>
                      )}
                      {e.status === "drafted" && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="shrink-0"
                          onClick={() =>
                            action(`approve-${e.contact_id}`, () =>
                              approveGroupContact(groupId, e.contact_id),
                            )
                          }
                          disabled={busy !== null}
                        >
                          {busy === `approve-${e.contact_id}` ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <>
                              <CheckCircle2 className="h-3.5 w-3.5 mr-1" /> Approve
                            </>
                          )}
                        </Button>
                      )}
                    </div>
                    {isOpen && e.conversation_id && (
                      <div className="border-t bg-muted/20 p-3">
                        {e.status === "drafted" ? (
                          <GroupDraftEditor
                            groupId={groupId}
                            contactId={e.contact_id}
                            conversationId={e.conversation_id}
                            onChanged={refresh}
                          />
                        ) : (
                          <ConversationThread conversationId={e.conversation_id} embedded />
                        )}
                      </div>
                    )}
                  </Card>
                );
              })}
            </div>
          )}
        </div>

        {/* Follow-up settings + notifications */}
        <div className="space-y-4">
          {group.notifications && group.notifications.length > 0 && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm flex items-center gap-2">
                  <Bell className="h-4 w-4" /> Needs attention ({group.notifications.length})
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                {group.notifications.map((t) => (
                  <div key={t.id} className="text-xs border rounded px-2 py-1">
                    {t.title}
                  </div>
                ))}
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm">If no reply — cadence</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {stepMsg && (
                <Alert>
                  <AlertDescription>{stepMsg}</AlertDescription>
                </Alert>
              )}
              <FollowupStepEditor
                steps={steps}
                onChange={(next) => {
                  setSteps(next);
                  setStepsDirty(true);
                }}
              />
              <div className="flex items-center gap-2">
                <Button onClick={saveSteps} disabled={savingSteps} size="sm">
                  {savingSteps ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                  Save cadence
                </Button>
                {stepsDirty && (
                  <span className="text-[11px] text-amber-600">unsaved changes</span>
                )}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm">When they reply</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="space-y-1">
                <label className="text-xs text-muted-foreground">If they reply positively</label>
                <select
                  value={onPos}
                  onChange={(e) => saveBranches(e.target.value, onNeg)}
                  className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
                >
                  <option value="auto_draft_reply">Auto-draft a reply for approval</option>
                  <option value="notify">Notify me to take over</option>
                </select>
              </div>
              <div className="space-y-1">
                <label className="text-xs text-muted-foreground">If they reply negatively</label>
                <select
                  value={onNeg}
                  onChange={(e) => saveBranches(onPos, e.target.value)}
                  className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
                >
                  <option value="mark_cold">Stop &amp; mark cold</option>
                  <option value="notify">Notify me to review</option>
                </select>
              </div>
              <p className="text-[11px] text-muted-foreground">
                The cadence above runs only while there&apos;s no reply. The moment they respond, it
                stops and this takes over.
              </p>

              <div className="space-y-2 pt-2 border-t">
                <div className="flex items-center justify-between">
                  <label className="text-xs font-medium">Reply playbook</label>
                  <div className="flex items-center gap-2">
                    {playbookDirty && (
                      <span className="text-[11px] text-amber-600">unsaved changes</span>
                    )}
                    <Button size="sm" variant="outline" onClick={savePlaybook} disabled={savingPlaybook}>
                      {savingPlaybook ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : "Save playbook"}
                    </Button>
                  </div>
                </div>
                <p className="text-[11px] text-muted-foreground">
                  Standing instructions per reply type — the AI follows them when drafting.
                </p>
                <PlaybookEditor
                  entries={playbook}
                  onChange={(next) => {
                    setPlaybook(next);
                    setPlaybookDirty(true);
                  }}
                />
                {playbookMsg && <p className="text-[11px] text-muted-foreground">{playbookMsg}</p>}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      <DraftAssistantDock groupId={groupId} onDraftsChanged={refresh} />
    </div>
  );
}
