"use client";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Plus, Trash2 } from "lucide-react";
import type { SequenceStep, SequenceTrigger, SequenceAction } from "@/lib/types/outreach";

const TRIGGERS: { value: SequenceTrigger; label: string; help: string }[] = [
  { value: "no_reply", label: "No reply", help: "Fires after wait_days if recipient hasn't replied" },
  { value: "any_reply", label: "Any reply", help: "Fires once any inbound reply lands" },
  { value: "positive_reply", label: "Positive reply", help: "Like any_reply but only when sentiment AI says positive" },
  { value: "always", label: "Always", help: "Fires after wait_days regardless of state" },
];

const ACTIONS: { value: SequenceAction; label: string; help: string }[] = [
  { value: "send_message", label: "Send message", help: "AI drafts a follow-up. Auto-sends if checked, else queued" },
  { value: "mark_cold", label: "Mark cold", help: "Stop the conversation — moves contact to lost" },
  { value: "advance_stage", label: "Advance stage", help: "Move pipeline_stage forward (uses prompt_override as new stage name)" },
  { value: "notify_user", label: "Notify user", help: "Log a notification (in-app surface coming later)" },
];

export interface Props {
  steps: SequenceStep[];
  onChange: (next: SequenceStep[]) => void;
}

export function SequenceStepEditor({ steps, onChange }: Props) {
  function update(i: number, patch: Partial<SequenceStep>) {
    const next = steps.map((s, idx) => (idx === i ? { ...s, ...patch } : s));
    onChange(next);
  }

  function remove(i: number) {
    const next = steps
      .filter((_, idx) => idx !== i)
      .map((s, idx) => ({ ...s, step_number: idx + 1 }));
    onChange(next);
  }

  function add() {
    const next = [
      ...steps,
      {
        step_number: steps.length + 1,
        wait_days: steps.length === 0 ? 0 : 3,
        trigger: "no_reply" as SequenceTrigger,
        action: "send_message" as SequenceAction,
        prompt_override: null,
        auto_send: false,
      },
    ];
    onChange(next);
  }

  return (
    <div className="space-y-3">
      {steps.length === 0 && (
        <p className="text-sm text-muted-foreground">No steps yet. Add the first one below.</p>
      )}
      {steps.map((step, i) => {
        const trigger = TRIGGERS.find((t) => t.value === step.trigger);
        const action = ACTIONS.find((a) => a.value === step.action);
        return (
          <div key={i} className="border rounded-md p-3 space-y-2 relative">
            <div className="flex items-center gap-2">
              <span className="font-medium text-sm">Step {i + 1}</span>
              <Button
                variant="ghost"
                size="icon"
                className="ml-auto h-7 w-7"
                onClick={() => remove(i)}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-2">
              <div className="space-y-1">
                <Label className="text-xs">Wait days</Label>
                <Input
                  type="number"
                  min={0}
                  value={step.wait_days}
                  onChange={(e) => update(i, { wait_days: Number(e.target.value) })}
                />
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Trigger</Label>
                <select
                  value={step.trigger}
                  onChange={(e) => update(i, { trigger: e.target.value as SequenceTrigger })}
                  className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
                >
                  {TRIGGERS.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">Action</Label>
                <select
                  value={step.action}
                  onChange={(e) => update(i, { action: e.target.value as SequenceAction })}
                  className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
                >
                  {ACTIONS.map((a) => (
                    <option key={a.value} value={a.value}>
                      {a.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            {step.action === "send_message" && (
              <label className="flex items-center gap-2 text-xs cursor-pointer">
                <input
                  type="checkbox"
                  checked={step.auto_send}
                  onChange={(e) => update(i, { auto_send: e.target.checked })}
                  className="h-3.5 w-3.5 accent-blue-600"
                />
                Auto-send (skip approval). Engine still requires &gt;=1 prior human-sent message.
              </label>
            )}
            {step.action === "advance_stage" && (
              <div className="space-y-1">
                <Label className="text-xs">New stage</Label>
                <Input
                  value={step.prompt_override ?? ""}
                  onChange={(e) => update(i, { prompt_override: e.target.value || null })}
                  placeholder="qualified"
                />
              </div>
            )}
            <p className="text-[11px] text-muted-foreground">
              {trigger?.help} → {action?.help}
            </p>
          </div>
        );
      })}
      <Button variant="outline" size="sm" onClick={add}>
        <Plus className="h-4 w-4 mr-1" /> Add step
      </Button>
    </div>
  );
}
