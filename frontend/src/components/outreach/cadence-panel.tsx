"use client";

import { useState } from "react";
import useSWR from "swr";
import { getGroupCadence } from "@/lib/api/outreach";
import { ChevronDown, ChevronRight, BarChart3 } from "lucide-react";

// CadencePanel shows follow-up sequence telemetry for a group: the per-touch
// funnel, completion %, why contacts exited, and what's queued to send next.
export function CadencePanel({ groupId }: { groupId: string }) {
  const [open, setOpen] = useState(false);
  const { data } = useSWR(open ? `/outreach/groups/${groupId}/cadence` : null, () =>
    getGroupCadence(groupId),
  );

  const maxSent = data ? Math.max(1, ...data.funnel.map((f) => f.sent)) : 1;
  const exitLabels: Record<string, string> = {
    replied_positive: "Replied — positive",
    replied_neutral: "Replied — neutral",
    replied_negative: "Replied — negative",
    cold: "Cold (no reply)",
    failed: "Send failed",
    in_progress: "Still in cadence",
  };

  return (
    <div className="rounded-md border">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-accent/50"
      >
        {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        <BarChart3 className="h-4 w-4 text-primary" />
        <span className="font-medium">Cadence progress</span>
        {data && (
          <span className="text-xs text-muted-foreground ml-auto">
            {data.completion_pct}% complete · {data.total_contacts} contacts
          </span>
        )}
      </button>

      {open && (
        <div className="border-t p-3 space-y-4 text-sm">
          {!data ? (
            <p className="text-xs text-muted-foreground">Loading…</p>
          ) : (
            <>
              {/* Funnel */}
              <div className="space-y-1.5">
                <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Touches sent</p>
                {data.funnel.map((f) => (
                  <div key={f.step} className="flex items-center gap-2">
                    <span className="w-24 shrink-0 text-xs truncate">{f.label}</span>
                    <div className="flex-1 h-4 rounded bg-muted overflow-hidden">
                      <div
                        className="h-full bg-primary/70"
                        style={{ width: `${(f.sent / maxSent) * 100}%` }}
                      />
                    </div>
                    <span className="w-16 shrink-0 text-right text-xs text-muted-foreground">
                      {f.sent} ({f.pct}%)
                    </span>
                  </div>
                ))}
              </div>

              {/* Exit breakdown */}
              <div className="space-y-1">
                <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Outcomes</p>
                <div className="flex flex-wrap gap-2">
                  {Object.entries(data.exits)
                    .filter(([, n]) => n > 0)
                    .map(([k, n]) => (
                      <span key={k} className="rounded-md border bg-muted/30 px-2 py-1 text-xs">
                        {exitLabels[k] ?? k.replace(/_/g, " ")}: <span className="font-medium">{n}</span>
                      </span>
                    ))}
                </div>
              </div>

              {/* Upcoming */}
              <div className="space-y-1">
                <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Upcoming sends</p>
                {data.upcoming.length === 0 ? (
                  <p className="text-xs text-muted-foreground">Nothing scheduled.</p>
                ) : (
                  <div className="space-y-0.5">
                    {data.upcoming.map((u) => (
                      <div key={u.date} className="flex items-center gap-2 text-xs">
                        <span className="w-24 shrink-0 text-muted-foreground">
                          {new Date(u.date).toLocaleDateString("en-GB", { weekday: "short", day: "2-digit", month: "short" })}
                        </span>
                        {u.cold > 0 && <span>{u.cold} cold</span>}
                        {u.followup > 0 && <span className="text-muted-foreground">{u.followup} follow-up</span>}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}
