"use client";

import Link from "next/link";
import useSWR from "swr";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { listSequences } from "@/lib/api/outreach";
import { Loader2, Plus, Workflow } from "lucide-react";

export default function SequencesPage() {
  const { data, isLoading } = useSWR("/outreach/sequences/", () => listSequences());

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    );
  }

  const sequences = data?.sequences ?? [];

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Sequences</h1>
          <p className="text-sm text-muted-foreground">
            Follow-up playbooks. Each step waits a few days, fires on reply state, drafts (or auto-sends) the next message.
          </p>
        </div>
        <Link href="/outreach/sequences/new">
          <Button>
            <Plus className="h-4 w-4 mr-1" /> New sequence
          </Button>
        </Link>
      </div>

      {sequences.length === 0 ? (
        <Card>
          <CardContent className="py-10 flex flex-col items-center text-center text-muted-foreground gap-2">
            <Workflow className="h-8 w-8" />
            <p>No sequences yet.</p>
            <Link href="/outreach/sequences/new">
              <Button variant="outline" size="sm">Build your first</Button>
            </Link>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-2">
          {sequences.map((s) => (
            <Link key={s.id} href={`/outreach/sequences/${s.id}`}>
              <Card className="hover:bg-accent/30 transition-colors cursor-pointer">
                <CardContent className="py-3 flex items-center gap-3">
                  <div className="flex-1 min-w-0">
                    <div className="font-medium truncate">{s.name}</div>
                    <div className="text-xs text-muted-foreground truncate">
                      {s.description || "No description"}
                    </div>
                  </div>
                  {s.is_template && <Badge variant="secondary">Template</Badge>}
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
