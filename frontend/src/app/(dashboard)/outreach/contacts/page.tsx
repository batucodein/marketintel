"use client";

import { useState } from "react";
import useSWR from "swr";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  listContactGroups,
  listContactGroupContacts,
  deleteContactGroup,
  setContactGroupBrand,
  listSenderProfiles,
  startConversation,
} from "@/lib/api/outreach";
import type { Contact, ContactGroup, SenderProfile } from "@/lib/types/outreach";
import { ChevronDown, ChevronRight, Loader2, Mail, Users, Trash2 } from "lucide-react";

const STAGE_COLORS: Record<string, string> = {
  lead: "bg-gray-100 text-gray-700",
  contacted: "bg-blue-100 text-blue-700",
  replied: "bg-green-100 text-green-700",
  qualified: "bg-amber-100 text-amber-700",
  won: "bg-emerald-100 text-emerald-700",
  lost: "bg-red-100 text-red-700",
};

export default function ContactsPage() {
  const { data, isLoading, mutate } = useSWR("/outreach/contact-groups", () => listContactGroups());
  const { data: profiles } = useSWR<SenderProfile[]>("/outreach/sender-profile", () =>
    listSenderProfiles(),
  );

  const groups = data?.groups ?? [];

  async function handleDelete(id: string) {
    if (!confirm("Delete this contact group? The contacts stay in your account; only the group is removed."))
      return;
    try {
      await deleteContactGroup(id);
      mutate();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading contact groups…
      </div>
    );
  }

  if (groups.length === 0) {
    return (
      <Card className="p-10 text-center">
        <Users className="h-10 w-10 mx-auto text-muted-foreground mb-3" />
        <h2 className="text-lg font-semibold mb-1">No contact groups yet</h2>
        <p className="text-sm text-muted-foreground max-w-sm mx-auto">
          Go to Markets → open a lead list → &quot;Add to contacts&quot; to create your first contact
          group. Email groups are built from contact groups.
        </p>
      </Card>
    );
  }

  return (
    <div className="space-y-3">
      <div>
        <h1 className="text-xl font-bold">Contacts</h1>
        <p className="text-sm text-muted-foreground">
          Contact groups are permanent collections you build from market leads, each with its own
          brand. Email groups are created from them.
        </p>
      </div>
      {groups.map((g) => (
        <ContactGroupSection
          key={g.id}
          group={g}
          profiles={profiles ?? []}
          onDelete={() => handleDelete(g.id)}
          onBrandChange={() => mutate()}
        />
      ))}
    </div>
  );
}

function ContactGroupSection({
  group,
  profiles,
  onDelete,
  onBrandChange,
}: {
  group: ContactGroup;
  profiles: SenderProfile[];
  onDelete: () => void;
  onBrandChange: () => void;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [busyID, setBusyID] = useState<string | null>(null);
  const { data } = useSWR(open ? [`/outreach/contact-groups/${group.id}/contacts`] : null, () =>
    listContactGroupContacts(group.id),
  );
  const contacts = data?.contacts ?? [];

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

  async function changeBrand(profileId: string) {
    try {
      await setContactGroupBrand(group.id, profileId || null);
      onBrandChange();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed to set brand");
    }
  }

  return (
    <div className="border rounded-md">
      <div className="flex items-center gap-2 px-3 py-2">
        <button onClick={() => setOpen((o) => !o)} className="flex items-center gap-2 min-w-0 flex-1 text-left">
          {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          <span className="font-medium truncate">{group.name}</span>
          <span className="text-xs text-muted-foreground">{group.member_count} contacts</span>
        </button>
        <select
          value={group.sender_profile_id ?? ""}
          onChange={(e) => changeBrand(e.target.value)}
          className="h-8 rounded-md border border-input bg-background px-2 text-xs max-w-[160px]"
          title="Brand this group sends as"
        >
          <option value="">No brand</option>
          {profiles.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name || p.company_name}
            </option>
          ))}
        </select>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-muted-foreground hover:text-red-600"
          onClick={onDelete}
          title="Delete contact group"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </div>
      {open && (
        <div className="p-2 pt-0 space-y-2">
          {contacts.length === 0 ? (
            <p className="text-sm text-muted-foreground px-1 py-2">No contacts in this group.</p>
          ) : (
            contacts.map((c) => (
              <Card key={c.id} className="p-3 flex items-center justify-between gap-3">
                <Link href={`/outreach/contacts/${c.id}`} className="min-w-0 flex-1 hover:underline">
                  <div className="flex items-center gap-2">
                    <span className="font-medium truncate">{c.display_name}</span>
                    <Badge className={STAGE_COLORS[c.pipeline_stage] ?? ""}>{c.pipeline_stage}</Badge>
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
            ))
          )}
        </div>
      )}
    </div>
  );
}
