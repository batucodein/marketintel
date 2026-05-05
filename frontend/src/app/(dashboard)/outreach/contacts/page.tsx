"use client";

import useSWR from "swr";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { listContacts, startConversation } from "@/lib/api/outreach";
import type { Contact } from "@/lib/types/outreach";
import { Loader2, Mail, Users } from "lucide-react";
import { useState } from "react";

const STAGE_COLORS: Record<string, string> = {
  lead: "bg-gray-100 text-gray-700",
  contacted: "bg-blue-100 text-blue-700",
  replied: "bg-green-100 text-green-700",
  qualified: "bg-amber-100 text-amber-700",
  won: "bg-emerald-100 text-emerald-700",
  lost: "bg-red-100 text-red-700",
};

export default function ContactsPage() {
  const router = useRouter();
  const { data, isLoading } = useSWR("/outreach/contacts", () => listContacts());
  const [busyID, setBusyID] = useState<string | null>(null);

  async function handleEmail(c: Contact) {
    if (!c.primary_email) {
      alert("No email on file for this contact.");
      return;
    }
    setBusyID(c.id);
    try {
      const res = await startConversation(c.id, true);
      router.push(`/outreach/${res.conversation.id}`);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to start conversation");
      setBusyID(null);
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading contacts…
      </div>
    );
  }

  const contacts = data?.contacts ?? [];

  if (contacts.length === 0) {
    return (
      <Card className="p-10 text-center">
        <Users className="h-10 w-10 mx-auto text-muted-foreground mb-3" />
        <h2 className="text-lg font-semibold mb-1">No contacts yet</h2>
        <p className="text-sm text-muted-foreground max-w-sm mx-auto">
          Contacts are created when you email a lead from a market. Go to Markets → open a lead
          list → click Email to start a conversation.
        </p>
      </Card>
    );
  }

  return (
    <div className="space-y-2">
      {contacts.map((c) => (
        <Card key={c.id} className="p-3 flex items-center justify-between gap-3">
          <Link href={`/outreach/contacts/${c.id}`} className="min-w-0 flex-1 hover:underline">
            <div className="flex items-center gap-2">
              <span className="font-medium truncate">{c.display_name}</span>
              <Badge className={STAGE_COLORS[c.pipeline_stage] ?? ""}>
                {c.pipeline_stage}
              </Badge>
              {c.unsubscribed_at && (
                <Badge variant="outline" className="text-red-600 border-red-300">
                  unsubscribed
                </Badge>
              )}
            </div>
            <div className="text-xs text-muted-foreground font-mono truncate">
              {c.primary_email ?? "no email"}
            </div>
          </Link>
          <Button
            size="sm"
            onClick={() => handleEmail(c)}
            disabled={busyID === c.id || !c.primary_email || !!c.unsubscribed_at}
          >
            {busyID === c.id ? (
              <Loader2 className="h-3 w-3 animate-spin mr-1" />
            ) : (
              <Mail className="h-3 w-3 mr-1" />
            )}
            Email
          </Button>
        </Card>
      ))}
    </div>
  );
}
