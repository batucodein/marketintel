"use client";

import { useRef, useState, useEffect } from "react";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  getSenderProfile,
  updateSenderProfile,
  uploadSenderCatalog,
  deleteSenderCatalog,
} from "@/lib/api/outreach";
import type { SenderProfile } from "@/lib/types/outreach";
import { Loader2, Paperclip, Trash2 } from "lucide-react";

const CATALOG_MAX_BYTES = 10 * 1024 * 1024;

function formatBytes(n: number | null | undefined): string {
  if (!n) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

export default function SenderProfilePage() {
  const { data, mutate, isLoading } = useSWR<SenderProfile>(
    "/outreach/sender-profile",
    () => getSenderProfile(),
  );
  const [form, setForm] = useState<Partial<SenderProfile>>({});
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [catalogBusy, setCatalogBusy] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (data) setForm(data);
  }, [data]);

  const field = (key: keyof SenderProfile) => (form[key] as string) ?? "";
  const set = (key: keyof SenderProfile, v: string) => {
    setForm((prev) => ({ ...prev, [key]: v }));
  };

  async function handleCatalogPick(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    if (file.size > CATALOG_MAX_BYTES) {
      setMsg("Catalog exceeds 10 MB limit");
      return;
    }
    setCatalogBusy(true);
    setMsg(null);
    try {
      const saved = await uploadSenderCatalog(file);
      setForm(saved);
      mutate(saved, { revalidate: false });
      setMsg(`Uploaded ${file.name}`);
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setCatalogBusy(false);
    }
  }

  async function handleCatalogDelete() {
    if (!confirm("Remove catalog attachment?")) return;
    setCatalogBusy(true);
    setMsg(null);
    try {
      await deleteSenderCatalog();
      const updated: Partial<SenderProfile> = {
        ...form,
        catalog_file_name: null,
        catalog_mime_type: null,
        catalog_size_bytes: null,
        catalog_uploaded_at: null,
      };
      setForm(updated);
      mutate(updated as SenderProfile, { revalidate: true });
      setMsg("Catalog removed");
    } catch (err) {
      setMsg(err instanceof Error ? err.message : "Remove failed");
    } finally {
      setCatalogBusy(false);
    }
  }

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

          <div className="space-y-1">
            <Label htmlFor="physical_address">
              Physical address <span className="text-muted-foreground text-xs">(required to launch any campaign — CAN-SPAM/GDPR)</span>
            </Label>
            <Textarea
              id="physical_address"
              value={field("physical_address")}
              onChange={(e) => set("physical_address", e.target.value)}
              placeholder={`ABC Mermer Sanayi A.Ş.\nOrg. San. Bölgesi, 5. Cadde No:12\n03200 Afyon, Türkiye`}
              rows={3}
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

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Product catalog</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">
            Upload a single PDF (up to 10 MB). When composing an email you can
            opt in to attach it — the AI draft will reference it naturally.
          </p>

          <input
            ref={fileInputRef}
            type="file"
            accept="application/pdf,.pdf"
            className="hidden"
            onChange={handleCatalogPick}
          />

          {form.catalog_file_name ? (
            <div className="flex items-center gap-3 rounded-md border px-3 py-2">
              <Paperclip className="h-4 w-4 text-muted-foreground" />
              <div className="flex-1 min-w-0">
                <div className="truncate text-sm font-medium">
                  {form.catalog_file_name}
                </div>
                <div className="text-xs text-muted-foreground">
                  {formatBytes(form.catalog_size_bytes)}
                  {form.catalog_uploaded_at &&
                    ` • uploaded ${new Date(form.catalog_uploaded_at).toLocaleDateString()}`}
                </div>
              </div>
              <Button
                size="sm"
                variant="outline"
                onClick={() => fileInputRef.current?.click()}
                disabled={catalogBusy}
              >
                Replace
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={handleCatalogDelete}
                disabled={catalogBusy}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ) : (
            <Button
              variant="outline"
              onClick={() => fileInputRef.current?.click()}
              disabled={catalogBusy}
            >
              {catalogBusy ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin mr-2" /> Uploading…
                </>
              ) : (
                <>
                  <Paperclip className="h-4 w-4 mr-2" /> Upload catalog PDF
                </>
              )}
            </Button>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
