"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import useSWR from "swr";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  createCampaign,
  addContactsToCampaign,
  listContacts,
  getSenderProfile,
  listSequences,
} from "@/lib/api/outreach";
import type { Contact, SenderProfile } from "@/lib/types/outreach";
import { ArrowLeft, Loader2, Megaphone } from "lucide-react";

export default function NewCampaignPage() {
  const router = useRouter();

  const [name, setName] = useState("");
  const [goal, setGoal] = useState("");
  const [pace, setPace] = useState(50);
  const [attachCatalog, setAttachCatalog] = useState(false);
  const [overridingProduct, setOverridingProduct] = useState("");
  const [overridingValueProp, setOverridingValueProp] = useState("");
  const [sequenceId, setSequenceId] = useState<string>("");
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { data: sequencesData } = useSWR("/outreach/sequences/", () => listSequences());
  const sequences = sequencesData?.sequences ?? [];

  const { data: profile } = useSWR<SenderProfile>("/outreach/sender-profile", () =>
    getSenderProfile(),
  );
  const hasCatalog =
    Boolean(profile?.catalog_file_name && (profile?.catalog_size_bytes ?? 0) > 0);
  const missingAddress = !profile?.physical_address;

  const { data: contactsData, isLoading: contactsLoading } = useSWR(
    "/outreach/contacts",
    () => listContacts(undefined, 1, 200),
  );

  const contacts = (contactsData?.contacts ?? []).filter((c) => c.primary_email);

  function toggle(id: string) {
    const next = new Set(selectedIDs);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSelectedIDs(next);
  }

  function toggleAll() {
    if (selectedIDs.size === contacts.length) setSelectedIDs(new Set());
    else setSelectedIDs(new Set(contacts.map((c) => c.id)));
  }

  async function handleSubmit() {
    if (!name) {
      setError("Campaign name is required.");
      return;
    }
    if (selectedIDs.size === 0) {
      setError("Select at least one contact.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const positioning =
        overridingProduct || overridingValueProp
          ? {
              product_description: overridingProduct,
              value_prop: overridingValueProp,
            }
          : undefined;
      const campaign = await createCampaign({
        name: name.trim(),
        goal: goal.trim(),
        send_pace_per_day: pace,
        attach_catalog: attachCatalog,
        positioning_override: positioning,
        sequence_id: sequenceId || null,
      });
      const result = await addContactsToCampaign(campaign.id, Array.from(selectedIDs));
      if (result.skipped_overlap?.length) {
        setError(
          `Created with ${result.added} contacts; ${result.skipped_overlap.length} skipped (already in another active conversation).`,
        );
      }
      // Brief delay so the user can read the warning if any.
      setTimeout(() => router.push(`/outreach/campaigns/${campaign.id}`), 600);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Create failed");
      setSubmitting(false);
    }
  }

  return (
    <div className="max-w-4xl space-y-4">
      <div className="flex items-center gap-2">
        <Link href="/outreach/campaigns">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Megaphone className="h-5 w-5" /> New campaign
          </h1>
          <p className="text-sm text-muted-foreground">
            Pick contacts, configure positioning, and the AI will draft each email asynchronously.
          </p>
        </div>
      </div>

      {missingAddress && (
        <Alert variant="destructive">
          <AlertDescription>
            Your sender profile is missing a physical address. CAN-SPAM/GDPR require it before
            you can launch a campaign.{" "}
            <Link href="/outreach/settings/profile" className="underline">
              Add it now
            </Link>
            .
          </AlertDescription>
        </Alert>
      )}

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Campaign details</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-1">
            <Label>Name *</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Q2 Marble Outreach – US"
            />
          </div>
          <div className="space-y-1">
            <Label>Goal</Label>
            <Input
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="Get a discovery call with US-based stone importers"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label>Send pace (per day)</Label>
              <Input
                type="number"
                min={1}
                max={500}
                value={pace}
                onChange={(e) => setPace(Number(e.target.value))}
              />
            </div>
            {hasCatalog && (
              <label className="flex items-center gap-2 text-sm pt-6 cursor-pointer">
                <input
                  type="checkbox"
                  checked={attachCatalog}
                  onChange={(e) => setAttachCatalog(e.target.checked)}
                  className="h-4 w-4 accent-blue-600"
                />
                Attach catalog ({profile?.catalog_file_name})
              </label>
            )}
          </div>
          {sequences.length > 0 && (
            <div className="space-y-1">
              <Label>Follow-up sequence (optional)</Label>
              <select
                value={sequenceId}
                onChange={(e) => setSequenceId(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="">No sequence — single send only</option>
                {sequences.map((s) => (
                  <option key={s.id} value={s.id}>{s.name}</option>
                ))}
              </select>
              <p className="text-xs text-muted-foreground">
                When attached, the engine starts a follow-up run on each conversation after the initial send.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Positioning override (optional)</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-xs text-muted-foreground">
            Leave empty to use your sender profile defaults. Override here when this campaign
            pitches a different product/angle.
          </p>
          <div className="space-y-1">
            <Label>Product description</Label>
            <Textarea
              rows={2}
              value={overridingProduct}
              onChange={(e) => setOverridingProduct(e.target.value)}
              placeholder="(uses sender profile if blank)"
            />
          </div>
          <div className="space-y-1">
            <Label>Value proposition</Label>
            <Textarea
              rows={2}
              value={overridingValueProp}
              onChange={(e) => setOverridingValueProp(e.target.value)}
              placeholder="(uses sender profile if blank)"
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">
              Pick contacts ({selectedIDs.size} selected)
            </CardTitle>
            {contacts.length > 0 && (
              <Button variant="ghost" size="sm" onClick={toggleAll}>
                {selectedIDs.size === contacts.length ? "Clear" : "Select all"}
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent>
          {contactsLoading ? (
            <div className="flex items-center gap-2 text-muted-foreground py-4">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading contacts…
            </div>
          ) : contacts.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No contacts with email yet. Click &quot;Email&quot; on any lead in a market to create one.
            </p>
          ) : (
            <div className="border rounded-md max-h-80 overflow-y-auto">
              {contacts.map((c: Contact) => (
                <label
                  key={c.id}
                  className="flex items-center gap-3 px-3 py-2 border-b last:border-b-0 cursor-pointer hover:bg-accent/30"
                >
                  <input
                    type="checkbox"
                    checked={selectedIDs.has(c.id)}
                    onChange={() => toggle(c.id)}
                    className="h-4 w-4 accent-blue-600"
                  />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium truncate">{c.display_name}</div>
                    <div className="text-xs text-muted-foreground truncate">
                      {c.primary_email} · {c.pipeline_stage}
                    </div>
                  </div>
                  {c.unsubscribed_at && (
                    <span className="text-xs text-red-600">unsubscribed</span>
                  )}
                </label>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <div className="flex justify-end gap-2">
        <Link href="/outreach/campaigns">
          <Button variant="outline">Cancel</Button>
        </Link>
        <Button
          onClick={handleSubmit}
          disabled={submitting || selectedIDs.size === 0 || !name}
        >
          {submitting ? (
            <>
              <Loader2 className="h-4 w-4 animate-spin mr-2" /> Creating…
            </>
          ) : (
            `Create draft (${selectedIDs.size})`
          )}
        </Button>
      </div>
    </div>
  );
}
