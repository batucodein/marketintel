"use client";

import Link from "next/link";
import useSWR from "swr";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { listCampaigns } from "@/lib/api/outreach";
import { Loader2, Plus, Megaphone } from "lucide-react";

export default function CampaignsPage() {
  const { data, isLoading } = useSWR("/outreach/campaigns/", () => listCampaigns());

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    );
  }

  const campaigns = data?.campaigns ?? [];

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Campaigns</h1>
          <p className="text-sm text-muted-foreground">
            Bulk outreach with shared positioning. AI drafts each lead, you approve, scheduler sends at your pace.
          </p>
        </div>
        <Link href="/outreach/campaigns/new">
          <Button>
            <Plus className="h-4 w-4 mr-1" /> New campaign
          </Button>
        </Link>
      </div>

      {campaigns.length === 0 ? (
        <Card>
          <CardContent className="py-10 flex flex-col items-center text-center text-muted-foreground gap-2">
            <Megaphone className="h-8 w-8" />
            <p>No campaigns yet.</p>
            <Link href="/outreach/campaigns/new">
              <Button variant="outline" size="sm">Create your first</Button>
            </Link>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-2">
          {campaigns.map((c) => (
            <Link key={c.id} href={`/outreach/campaigns/${c.id}`}>
              <Card className="hover:bg-accent/30 transition-colors cursor-pointer">
                <CardContent className="py-3 flex items-center gap-3">
                  <div className="flex-1 min-w-0">
                    <div className="font-medium truncate">{c.name}</div>
                    <div className="text-xs text-muted-foreground truncate">
                      {c.goal || "No goal set"} · pace {c.send_pace_per_day}/day
                    </div>
                  </div>
                  <Badge variant={c.status === "active" ? "default" : "outline"}>
                    {c.status}
                  </Badge>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
