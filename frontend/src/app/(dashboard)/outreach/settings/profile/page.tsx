"use client";

import { useState, useEffect } from "react";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { getSenderProfile, updateSenderProfile } from "@/lib/api/outreach";
import type { SenderProfile } from "@/lib/types/outreach";
import { Loader2 } from "lucide-react";

export default function SenderProfilePage() {
  const { data, mutate, isLoading } = useSWR<SenderProfile>(
    "/outreach/sender-profile",
    () => getSenderProfile(),
  );
  const [form, setForm] = useState<Partial<SenderProfile>>({});
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  useEffect(() => {
    if (data) setForm(data);
  }, [data]);

  const field = (key: keyof SenderProfile) => (form[key] as string) ?? "";
  const set = (key: keyof SenderProfile, v: string) => {
    setForm((prev) => ({ ...prev, [key]: v }));
  };

  async function handleSave() {
    if (!form.company_name) {
      setMsg("Company name is required");
      return;
    }
    setSaving(true);
    setMsg(null);
    try {
      const saved = await updateSenderProfile(form);
      setForm(saved);
      mutate(saved, { revalidate: false });
      setMsg("Saved");
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    );
  }

  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <h1 className="text-2xl font-bold">Sender profile</h1>
        <p className="text-sm text-muted-foreground">
          These fields feed the AI when it drafts emails so your outreach sounds like you.
        </p>
      </div>

      {msg && (
        <Alert>
          <AlertDescription>{msg}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Your company</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="company_name">Company name *</Label>
            <Input
              id="company_name"
              value={field("company_name")}
              onChange={(e) => set("company_name", e.target.value)}
              placeholder="ABC Mermer Sanayi"
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="product_description">Product description</Label>
            <Textarea
              id="product_description"
              value={field("product_description")}
              onChange={(e) => set("product_description", e.target.value)}
              placeholder="Premium Calacatta and Carrara marble blocks and slabs from our Afyon quarry. Export-ready packaging, container pricing."
              rows={3}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="value_prop">Value proposition</Label>
            <Textarea
              id="value_prop"
              value={field("value_prop")}
              onChange={(e) => set("value_prop", e.target.value)}
              placeholder="Direct-from-quarry pricing, 2-week lead time via Mersin port, ISO 9001 certified."
              rows={2}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="target_buyer_description">Who are your best buyers?</Label>
            <Textarea
              id="target_buyer_description"
              value={field("target_buyer_description")}
              onChange={(e) => set("target_buyer_description", e.target.value)}
              placeholder="US-based kitchen remodeling contractors, mid-sized stone importers, architects sourcing premium finishes."
              rows={2}
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-1">
              <Label htmlFor="tone">Tone</Label>
              <Input
                id="tone"
                value={field("tone")}
                onChange={(e) => set("tone", e.target.value)}
                placeholder="formal, technical, direct"
              />
            </div>
          </div>

          <div className="space-y-1">
            <Label htmlFor="signature">Signature</Label>
            <Textarea
              id="signature"
              value={field("signature")}
              onChange={(e) => set("signature", e.target.value)}
              placeholder={`Batuhan Özalhan\nExport Director\nABC Mermer Sanayi\n+90 555 000 0000`}
              rows={5}
            />
          </div>

          <Button onClick={handleSave} disabled={saving}>
            {saving ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" /> Saving…
              </>
            ) : (
              "Save profile"
            )}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
