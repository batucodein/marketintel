"use client";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Trash2, Plus } from "lucide-react";
import type { PlaybookEntry } from "@/lib/types/outreach";
import { INTENT_TAGS } from "./intent-tags";

// PlaybookEditor is a controlled list of (intent tag → standing instruction)
// rows. The instruction is injected into the AI's draft whenever a reply with
// that tag is answered. Mirrors the FollowupStepEditor pattern.
export function PlaybookEditor({
  entries,
  onChange,
}: {
  entries: PlaybookEntry[];
  onChange: (next: PlaybookEntry[]) => void;
}) {
  const usedTags = new Set(entries.map((e) => e.tag));
  const firstFree = INTENT_TAGS.find((t) => !usedTags.has(t.value))?.value;

  function update(i: number, patch: Partial<PlaybookEntry>) {
    onChange(entries.map((e, j) => (j === i ? { ...e, ...patch } : e)));
  }

  return (
    <div className="space-y-2">
      {entries.length === 0 && (
        <p className="text-xs text-muted-foreground">
          No playbook yet — add a rule like: if they ask price → &quot;attach the price list and
          stress MOQ flexibility&quot;.
        </p>
      )}
      {entries.map((e, i) => (
        <div key={i} className="rounded-md border p-2 space-y-1.5">
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground shrink-0">If reply is</span>
            <select
              value={e.tag}
              onChange={(ev) => update(i, { tag: ev.target.value })}
              className="h-8 rounded-md border border-input bg-background px-2 text-xs"
            >
              {INTENT_TAGS.map((t) => (
                <option key={t.value} value={t.value} disabled={t.value !== e.tag && usedTags.has(t.value)}>
                  {t.label}
                </option>
              ))}
            </select>
            <Button
              variant="ghost"
              size="icon"
              className="ml-auto h-7 w-7 text-muted-foreground hover:text-red-600"
              onClick={() => onChange(entries.filter((_, j) => j !== i))}
              title="Remove rule"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
          <Textarea
            placeholder="What should the AI do in its reply? e.g. 'Attach the price list, quote FOB terms, and offer a call this week.'"
            value={e.instruction}
            onChange={(ev) => update(i, { instruction: ev.target.value })}
            rows={2}
            className="text-sm"
          />
          {e.instruction.trim() === "" && (
            <p className="text-xs text-amber-600">Empty rules aren&apos;t saved.</p>
          )}
        </div>
      ))}
      <Button
        variant="outline"
        size="sm"
        disabled={!firstFree}
        onClick={() => firstFree && onChange([...entries, { tag: firstFree, instruction: "" }])}
      >
        <Plus className="h-3.5 w-3.5 mr-1" /> Add rule
      </Button>
    </div>
  );
}
