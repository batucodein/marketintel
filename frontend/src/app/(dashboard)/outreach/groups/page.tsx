"use client";

import Link from "next/link";
import useSWR from "swr";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { listGroups } from "@/lib/api/outreach";
import type { GroupSummary } from "@/lib/types/outreach";
import { useOutreachEvents } from "@/lib/sse/use-outreach-events";
import { Layers, Loader2 } from "lucide-react";

// Derive a human status from the per-bucket counts. Drafting/sending happens
// server-side on the scheduler tick, so this reflects live progress whenever
// the user lands here.
function statusFor(g: GroupSummary): { label: string; tone: string } {
  if (g.total_count === 0) return { label: "No contacts", tone: "text-muted-foreground" };
  if (g.pending_count > 0)
    return {
      label: `Drafting ${g.total_count - g.pending_count}/${g.total_count}`,
      tone: "text-amber-700 border-amber-300 bg-amber-50",
    };
  if (g.drafted_count > 0)
    return { label: `${g.drafted_count} awaiting approval`, tone: "text-blue-700 border-blue-300 bg-blue-50" };
  if (g.sent_count > 0 || g.replied_count > 0)
    return {
      label:
        `${g.sent_count} sent · ${g.replied_count} replied` +
        (g.cold_count > 0 ? ` · ${g.cold_count} cold` : ""),
      tone: "text-green-700 border-green-300 bg-green-50",
    };
  if (g.approved_count > 0)
    return { label: `${g.approved_count} scheduled`, tone: "text-blue-700 border-blue-300 bg-blue-50" };
  if (g.cold_count > 0)
    return { label: `${g.cold_count} cold`, tone: "text-muted-foreground" };
  return { label: "Ready", tone: "text-muted-foreground" };
}

export default function GroupsIndexPage() {
  const { data, isLoading, mutate } = useSWR<{ groups: GroupSummary[] }>(
    "/outreach/groups",
    () => listGroups(),
    { refreshInterval: 8000 },
  );

  // New inbound (a reply) can change a group's counts — refresh on push too.
  useOutreachEvents((e) => {
    if (e.kind === "inbound") mutate();
  });

  const groups = data?.groups ?? [];

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading groups…
      </div>
    );
  }

  if (groups.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center text-center text-muted-foreground py-20">
        <Layers className="h-10 w-10 mb-3" />
        <h2 className="text-lg font-semibold text-foreground mb-1">No email groups yet</h2>
        <p className="text-sm max-w-sm">
          Create one from the inbox: pick a market, its brand sends the emails, and the AI drafts
          and follows up automatically.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <h1 className="text-xl font-bold mb-2">Email groups</h1>
      {groups.map((g) => {
        const status = statusFor(g);
        return (
          <Link key={g.id} href={`/outreach/groups/${g.id}`}>
            <Card className="p-3 hover:border-blue-300 hover:shadow-sm transition cursor-pointer">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="font-medium truncate">{g.name}</div>
                  <div className="text-xs text-muted-foreground truncate">
                    {g.brand_name || "no brand"}
                    {g.contact_group_name ? ` · ${g.contact_group_name}` : ""} · {g.total_count} contacts
                  </div>
                </div>
                <Badge variant="outline" className={"text-[11px] whitespace-nowrap " + status.tone}>
                  {status.label}
                </Badge>
              </div>
            </Card>
          </Link>
        );
      })}
    </div>
  );
}
