"use client";

import Link from "next/link";
import useSWR from "swr";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { listConversations } from "@/lib/api/outreach";
import { Inbox as InboxIcon, Loader2 } from "lucide-react";

export default function OutreachInboxPage() {
  const { data, isLoading } = useSWR(
    "/outreach/conversations",
    () => listConversations(false, 1, 50),
    { refreshInterval: 30000 }, // refresh every 30s for new replies
  );

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading inbox…
      </div>
    );
  }

  const convs = data?.conversations ?? [];

  if (convs.length === 0) {
    return (
      <Card className="p-10 text-center">
        <InboxIcon className="h-10 w-10 mx-auto text-muted-foreground mb-3" />
        <h2 className="text-lg font-semibold mb-1">Your inbox is empty</h2>
        <p className="text-sm text-muted-foreground max-w-sm mx-auto">
          Start a conversation with a lead from any market to see it here. Replies from your
          recipients appear automatically within 2 minutes.
        </p>
      </Card>
    );
  }

  return (
    <div className="space-y-2">
      {convs.map((c) => {
        const last = c.last_message_at ? new Date(c.last_message_at) : null;
        return (
          <Link key={c.id} href={`/outreach/${c.id}`}>
            <Card
              className={
                "p-3 hover:border-blue-300 hover:shadow-sm transition cursor-pointer " +
                (c.unread ? "border-l-4 border-l-blue-600" : "")
              }
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className={"text-sm " + (c.unread ? "font-semibold" : "font-medium")}>
                      {c.contact_name}
                    </span>
                    <span className="text-xs text-muted-foreground truncate">
                      {c.business_name}
                    </span>
                    {c.unread && <Badge className="text-[10px]">New</Badge>}
                  </div>
                  {c.subject && (
                    <div className="text-xs text-muted-foreground mt-0.5 truncate">
                      {c.subject}
                    </div>
                  )}
                  {c.last_message_snippet && (
                    <div className="text-xs text-muted-foreground mt-1 truncate">
                      {c.last_direction === "out" ? "You: " : ""}
                      {c.last_message_snippet}
                    </div>
                  )}
                </div>
                <div className="text-[11px] text-muted-foreground shrink-0 text-right">
                  {last && last.toLocaleDateString("en-GB", { day: "numeric", month: "short" })}
                  {last && (
                    <div>
                      {last.toLocaleTimeString(undefined, {
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </div>
                  )}
                </div>
              </div>
            </Card>
          </Link>
        );
      })}
    </div>
  );
}
