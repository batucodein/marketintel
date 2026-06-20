import type { SequenceTrigger, SequenceAction } from "@/lib/types/outreach";

// Shared labels/help for follow-up triggers and the four actions. Consumed by
// the follow-up step editor and the timeline so they never drift.

// A cadence step only ever runs while the recipient is silent — the moment any
// reply lands it's intercepted at the campaign level (the "When they reply"
// branch), so reply-based triggers can never fire here. The dropdown is kept as
// a single-option shell for future engagement triggers (opened / clicked).
export const TRIGGERS: { value: SequenceTrigger; label: string; help: string }[] = [
  { value: "no_reply", label: "No reply", help: "Fires after the wait if the recipient hasn't replied" },
];

export const ACTIONS: { value: SequenceAction; label: string; help: string }[] = [
  { value: "send_message", label: "Send message", help: "AI drafts a follow-up. Auto-sends if enabled, else queued for approval" },
  { value: "mark_cold", label: "Mark cold", help: "Stop the conversation — moves the contact to cold" },
  { value: "advance_stage", label: "Advance stage", help: "Move the pipeline stage forward" },
  { value: "notify_user", label: "Notify me", help: "Surface a notification in the group view" },
];

export function actionLabel(value: SequenceAction): string {
  return ACTIONS.find((a) => a.value === value)?.label ?? value;
}

export function triggerLabel(value: SequenceTrigger): string {
  return TRIGGERS.find((t) => t.value === value)?.label ?? value;
}
