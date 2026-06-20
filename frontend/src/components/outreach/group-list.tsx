"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import useSWR from "swr";
import { listGroups } from "@/lib/api/outreach";
import type { GroupSummary } from "@/lib/types/outreach";
import { Loader2, Layers } from "lucide-react";

// GroupList is the persistent left rail. Lives in the groups layout so its
// state survives navigation between a group and its conversations.
export function GroupList() {
  const pathname = usePathname();
  const { data, isLoading } = useSWR<{ groups: GroupSummary[] }>(
    "/outreach/groups",
    () => listGroups(),
    { refreshInterval: 15000 },
  );

  const groups = data?.groups ?? [];

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2 px-2 py-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        <Layers className="h-3.5 w-3.5" /> Email Groups
      </div>
      {isLoading ? (
        <div className="flex items-center gap-2 text-muted-foreground p-2 text-sm">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : groups.length === 0 ? (
        <p className="px-2 py-2 text-xs text-muted-foreground">
          No groups yet. Create one from the inbox.
        </p>
      ) : (
        groups.map((g) => {
          const active = pathname.startsWith(`/outreach/groups/${g.id}`);
          return (
            <Link
              key={g.id}
              href={`/outreach/groups/${g.id}`}
              className={
                "block rounded-md border px-3 py-2 text-sm transition " +
                (active
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border hover:border-primary/50")
              }
            >
              <div className="font-medium truncate">{g.name}</div>
              <div className="text-xs text-muted-foreground truncate">
                {g.brand_name || "no brand"}
                {g.contact_group_name ? ` · ${g.contact_group_name}` : ""}
              </div>
              <div className="text-[11px] text-muted-foreground mt-0.5">
                {g.sent_count}/{g.total_count} sent · {g.replied_count} replied
              </div>
            </Link>
          );
        })
      )}
    </div>
  );
}
