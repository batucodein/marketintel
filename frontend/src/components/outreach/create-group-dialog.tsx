"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  listContactGroups,
  listContactGroupContacts,
  listSenderProfiles,
  listChannels,
  createGroup,
  saveGroupPlaybook,
} from "@/lib/api/outreach";
import type {
  Contact,
  ContactGroup,
  PlaybookEntry,
  SenderProfile,
  SequenceStep,
  UserChannel,
} from "@/lib/types/outreach";
import { FollowupStepEditor } from "@/components/outreach/followup-step-editor";
import { PlaybookEditor } from "@/components/outreach/playbook-editor";
import { Loader2 } from "lucide-react";

// CreateGroupDialog: contact group → brand (from the group) → name/goal +
// follow-up steps. The group's members become the email group's recipients.
export function CreateGroupDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const router = useRouter();
  const { data: cgData } = useSWR(open ? "/outreach/contact-groups" : null, () =>
    listContactGroups(),
  );
  const { data: profiles } = useSWR<SenderProfile[]>(
    open ? "/outreach/sender-profile" : null,
    () => listSenderProfiles(),
  );
  const { data: channelsData } = useSWR<UserChannel[]>(open ? "/outreach/channels" : null, () =>
    listChannels(),
  );
  const channels = (channelsData ?? []).filter((c) => c.enabled);

  const [groupId, setGroupId] = useState("");
  const [channelId, setChannelId] = useState("");
  const [name, setName] = useState("");
  const [goal, setGoal] = useState("");
  const [steps, setSteps] = useState<SequenceStep[]>([
    { step_number: 1, wait_days: 0, trigger: "no_reply", action: "send_message", prompt_override: null, auto_send: false },
  ]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [attach, setAttach] = useState(true);
  const [onPos, setOnPos] = useState("auto_draft_reply");
  const [onNeg, setOnNeg] = useState("mark_cold");
  const [showPlaybook, setShowPlaybook] = useState(false);
  const [playbook, setPlaybook] = useState<PlaybookEntry[]>([]);

  // Default the "send from" account to the user's default channel (else first).
  useEffect(() => {
    if (!channelId && channels.length > 0) {
      setChannelId((channels.find((c) => c.is_default) ?? channels[0]).id);
    }
  }, [channels, channelId]);

  const contactGroups = cgData?.groups ?? [];
  const cg = contactGroups.find((g) => g.id === groupId);
  const brand = profiles?.find((p) => p.id === cg?.sender_profile_id);
  const brandMissing = Boolean(cg) && !cg?.sender_profile_id;
  const brandHasAttachment = Boolean(brand?.catalog_file_name);

  const { data: membersData } = useSWR(
    open && groupId ? [`/outreach/contact-groups/${groupId}/contacts`] : null,
    () => listContactGroupContacts(groupId),
  );
  const members = membersData?.contacts ?? [];

  async function submit() {
    if (!name.trim() || !groupId) {
      setError("Pick a contact group and name the email group.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const group = await createGroup({
        name: name.trim(),
        goal: goal.trim(),
        contact_group_id: groupId,
        attach_catalog: brandHasAttachment && attach,
        on_positive_action: onPos,
        on_negative_action: onNeg,
        steps,
        channel_id: channelId || undefined,
      });
      if (playbook.some((p) => p.instruction.trim() !== "")) {
        // Best-effort: a playbook save failure shouldn't block landing on the
        // new group — the user can re-add rules from the group page.
        try {
          await saveGroupPlaybook(group.id, playbook);
        } catch (err) {
          console.error("Failed to save reply playbook", err);
        }
      }
      onOpenChange(false);
      router.push(`/outreach/groups/${group.id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create group");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full sm:max-w-2xl max-h-[85vh] overflow-y-auto overflow-x-hidden">
        <DialogHeader>
          <DialogTitle>New email group</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {/* Contact group */}
          <div className="space-y-1">
            <Label>Contact group</Label>
            <select
              value={groupId}
              onChange={(e) => setGroupId(e.target.value)}
              className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
            >
              <option value="">— Select a contact group —</option>
              {contactGroups.map((g: ContactGroup) => (
                <option key={g.id} value={g.id}>
                  {g.name} ({g.member_count})
                </option>
              ))}
            </select>
            {contactGroups.length === 0 && (
              <p className="text-xs text-muted-foreground">
                No contact groups yet — create one from a market&apos;s leads (Markets → a market →
                View Leads → Add to contacts).
              </p>
            )}
          </div>

          {/* Brand (from the contact group) */}
          {cg && (
            <div className="text-sm">
              {brandMissing ? (
                <Alert variant="destructive">
                  <AlertDescription>
                    This contact group has no brand. Set one on the Contacts page before creating
                    an email group.
                  </AlertDescription>
                </Alert>
              ) : (
                <p className="text-muted-foreground">
                  Sends as brand:{" "}
                  <span className="font-medium text-foreground">
                    {brand?.name ?? brand?.company_name ?? "…"}
                  </span>
                </p>
              )}
            </div>
          )}

          {/* Send-from account */}
          <div className="space-y-1">
            <Label>Send from</Label>
            {channels.length === 0 ? (
              <Alert variant="destructive">
                <AlertDescription>
                  No email account connected. Connect one in Settings → Email channels first.
                </AlertDescription>
              </Alert>
            ) : (
              <select
                value={channelId}
                onChange={(e) => setChannelId(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                {channels.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.display_label || c.from_email}
                  </option>
                ))}
              </select>
            )}
          </div>

          {/* Members preview */}
          {cg && !brandMissing && (
            <div className="text-sm text-muted-foreground">
              {members.length} contact{members.length !== 1 ? "s" : ""} from this group will be added.
              {members.length > 0 && (
                <div className="mt-1 max-h-28 overflow-y-auto rounded border p-2 space-y-0.5">
                  {members.slice(0, 50).map((c: Contact) => (
                    <div key={c.id} className="text-xs truncate">
                      {c.display_name}
                      {c.primary_email ? ` · ${c.primary_email}` : " · (no email — will skip)"}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Name + goal */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label>Group name</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="US marble importers Q3" />
            </div>
            <div className="space-y-1">
              <Label>Goal (optional)</Label>
              <Input value={goal} onChange={(e) => setGoal(e.target.value)} placeholder="Book intro calls" />
            </div>
          </div>

          {/* Attachment */}
          {cg && !brandMissing && brandHasAttachment && (
            <label className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="checkbox"
                checked={attach}
                onChange={(e) => setAttach(e.target.checked)}
                className="h-4 w-4 accent-blue-600"
              />
              Attach <span className="font-medium">{brand?.catalog_file_name}</span> to every email
            </label>
          )}

          {/* No-reply cadence */}
          <div className="space-y-2">
            <Label>If no reply — cadence</Label>
            <FollowupStepEditor steps={steps} onChange={setSteps} />
          </div>

          {/* Reply branches */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label>If they reply positively</Label>
              <select
                value={onPos}
                onChange={(e) => setOnPos(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="auto_draft_reply">Auto-draft a reply for approval</option>
                <option value="notify">Notify me to take over</option>
              </select>
            </div>
            <div className="space-y-1">
              <Label>If they reply negatively</Label>
              <select
                value={onNeg}
                onChange={(e) => setOnNeg(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="mark_cold">Stop &amp; mark cold</option>
                <option value="notify">Notify me to review</option>
              </select>
            </div>
          </div>

          {/* Reply playbook (optional) */}
          {!showPlaybook ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowPlaybook(true)}
            >
              + Add reply playbook
            </Button>
          ) : (
            <div className="space-y-2">
              <Label>Reply playbook (optional)</Label>
              <PlaybookEditor entries={playbook} onChange={setPlaybook} />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={submitting}>
            Cancel
          </Button>
          <Button
            onClick={submit}
            disabled={submitting || brandMissing || !groupId || !name.trim() || channels.length === 0}
          >
            {submitting ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
            Create group
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
