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
  createSenderProfile,
  updateSenderProfile,
  uploadSenderCatalog,
  deleteSenderCatalog,
  listBrandLessons,
  deleteBrandLesson,
} from "@/lib/api/outreach";
import type { SenderProfile } from "@/lib/types/outreach";
import { TagChip } from "@/components/outreach/tag-chip";
import { Loader2, Paperclip, Trash2 } from "lucide-react";

const CATALOG_MAX_BYTES = 10 * 1024 * 1024;

function formatBytes(n: number | null | undefined): string {
  if (!n) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

// SenderProfileEditor edits one brand (sender profile). profileId === null is
// create mode (catalog upload is disabled until the brand is saved once).
export function SenderProfileEditor({
  profileId,
  onSaved,
}: {
  profileId: string | null;
  onSaved?: (p: SenderProfile) => void;
}) {
  const { data, mutate, isLoading } = useSWR<SenderProfile | null>(
    profileId ? `/outreach/sender-profile/${profileId}` : null,
    () => getSenderProfile(profileId as string),
  );
  const [form, setForm] = useState<Partial<SenderProfile>>({ tone: "formal", name: "" });
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);
  const [catalogBusy, setCatalogBusy] = useState(false);
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (profileId && data) setForm(data);
    if (!profileId) setForm({ tone: "formal", name: "" });
  }, [data, profileId]);

  const field = (key: keyof SenderProfile) => (form[key] as string) ?? "";
  const set = (key: keyof SenderProfile, v: string) => setForm((prev) => ({ ...prev, [key]: v }));

  async function handleCatalogPick(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file || !profileId) return;
    if (file.size > CATALOG_MAX_BYTES) {
      setMsg("Catalog exceeds 10 MB limit");
      return;
    }
    setCatalogBusy(true);
    setMsg(null);
    try {
      const saved = await uploadSenderCatalog(profileId, file);
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
    if (!profileId || !confirm("Remove this attachment?")) return;
    setCatalogBusy(true);
    setMsg(null);
    try {
      await deleteSenderCatalog(profileId);
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
      const saved = profileId
        ? await updateSenderProfile(profileId, form)
        : await createSenderProfile(form);
      setForm(saved);
      mutate(saved, { revalidate: false });
      setMsg("Saved");
      onSaved?.(saved);
    } catch (e) {
      setMsg(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  if (profileId && isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {msg && (
        <Alert>
          <AlertDescription>{msg}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Brand</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="name">Brand name *</Label>
            <Input
              id="name"
              value={field("name")}
              onChange={(e) => set("name", e.target.value)}
              placeholder="Premium line / US market"
            />
            <p className="text-xs text-muted-foreground">
              An internal label so you can tell your brands apart (e.g. by market).
            </p>
          </div>

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
              rows={3}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="value_prop">Value proposition</Label>
            <Textarea
              id="value_prop"
              value={field("value_prop")}
              onChange={(e) => set("value_prop", e.target.value)}
              rows={2}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="target_buyer_description">Who are your best buyers?</Label>
            <Textarea
              id="target_buyer_description"
              value={field("target_buyer_description")}
              onChange={(e) => set("target_buyer_description", e.target.value)}
              rows={2}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="tone">Tone</Label>
            <Input
              id="tone"
              value={field("tone")}
              onChange={(e) => set("tone", e.target.value)}
              placeholder="formal, technical, direct"
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="signature">Signature</Label>
            <Textarea
              id="signature"
              value={field("signature")}
              onChange={(e) => set("signature", e.target.value)}
              rows={5}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="physical_address">
              Physical address{" "}
              <span className="text-muted-foreground text-xs">
                (required to launch — CAN-SPAM/GDPR)
              </span>
            </Label>
            <Textarea
              id="physical_address"
              value={field("physical_address")}
              onChange={(e) => set("physical_address", e.target.value)}
              rows={3}
            />
          </div>

          <Button onClick={handleSave} disabled={saving}>
            {saving ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" /> Saving…
              </>
            ) : profileId ? (
              "Save brand"
            ) : (
              "Create brand"
            )}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Targeting & ICP</CardTitle>
          <p className="text-xs text-muted-foreground">
            Used by the AI lead scorer. Free text — write naturally.
          </p>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="target_industries">Target industries</Label>
            <Textarea
              id="target_industries"
              value={field("target_industries")}
              onChange={(e) => set("target_industries", e.target.value)}
              rows={2}
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-1">
              <Label htmlFor="target_countries">Target countries</Label>
              <Input
                id="target_countries"
                value={field("target_countries")}
                onChange={(e) => set("target_countries", e.target.value)}
                placeholder="US, Canada, UK"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="avoid_countries">Countries to avoid</Label>
              <Input
                id="avoid_countries"
                value={field("avoid_countries")}
                onChange={(e) => set("avoid_countries", e.target.value)}
              />
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="space-y-1">
              <Label htmlFor="min_deal_size_usd">Min deal size (USD)</Label>
              <Input
                id="min_deal_size_usd"
                type="number"
                min={0}
                value={(form.min_deal_size_usd as number | null | undefined) ?? ""}
                onChange={(e) =>
                  setForm((prev) => ({
                    ...prev,
                    min_deal_size_usd: e.target.value === "" ? null : Number(e.target.value),
                  }))
                }
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="typical_deal_size_usd">Typical deal size (USD)</Label>
              <Input
                id="typical_deal_size_usd"
                type="number"
                min={0}
                value={(form.typical_deal_size_usd as number | null | undefined) ?? ""}
                onChange={(e) =>
                  setForm((prev) => ({
                    ...prev,
                    typical_deal_size_usd: e.target.value === "" ? null : Number(e.target.value),
                  }))
                }
              />
            </div>
          </div>

          <div className="space-y-1">
            <Label htmlFor="deal_breakers">Deal breakers</Label>
            <Textarea
              id="deal_breakers"
              value={field("deal_breakers")}
              onChange={(e) => set("deal_breakers", e.target.value)}
              rows={2}
            />
          </div>

          <div className="space-y-1">
            <Label htmlFor="competitive_moats">Competitive moats</Label>
            <Textarea
              id="competitive_moats"
              value={field("competitive_moats")}
              onChange={(e) => set("competitive_moats", e.target.value)}
              rows={2}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Attachment</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {!profileId ? (
            <p className="text-sm text-muted-foreground">
              Save the brand first, then upload its attachment.
            </p>
          ) : (
            <>
              <p className="text-sm text-muted-foreground">
                A single PDF (up to 10 MB) for this brand — your catalog, product sheet, or
                line card. Email groups using this brand can attach it to every email.
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
                    <div className="truncate text-sm font-medium">{form.catalog_file_name}</div>
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
                  <Button size="sm" variant="ghost" onClick={handleCatalogDelete} disabled={catalogBusy}>
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
                      <Paperclip className="h-4 w-4 mr-2" /> Upload attachment (PDF)
                    </>
                  )}
                </Button>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {profileId && <LearnedLessonsCard brandId={profileId} />}
    </div>
  );
}

// LearnedLessonsCard lists the brand's remembered reply lessons (saved via the
// "Remember this for this brand" toggle when adjusting a draft) so the user can
// review and delete them.
function LearnedLessonsCard({ brandId }: { brandId: string }) {
  const { data, mutate } = useSWR(`/outreach/sender-profile/${brandId}/lessons`, () =>
    listBrandLessons(brandId),
  );
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const lessons = data?.lessons ?? [];

  async function remove(id: string) {
    setDeleteError(null);
    try {
      await deleteBrandLesson(brandId, id);
      mutate();
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Failed to delete lesson");
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Learned reply lessons</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="text-sm text-muted-foreground">
          Saved when you adjust a draft and tick &quot;remember&quot;. Each lesson only applies to
          future drafts answering the same kind of reply (matched by sentiment + intent).
        </p>
        <p className="text-xs text-muted-foreground">
          Lessons saved from a conversation appear here after a refresh.
        </p>
        {deleteError && <p className="text-xs text-red-600">{deleteError}</p>}
        {lessons.length === 0 ? (
          <p className="text-xs text-muted-foreground">Nothing learned yet.</p>
        ) : (
          lessons.map((l) => (
            <div key={l.id} className="flex items-start gap-2 rounded-md border px-3 py-2">
              <div className="flex-1 min-w-0">
                <div className="text-sm">{l.instruction}</div>
                <div className="flex flex-wrap items-center gap-1 mt-1">
                  <span className="text-[10px] text-muted-foreground uppercase tracking-wide">
                    {l.match_sentiment} replies
                  </span>
                  {l.match_tags.map((t) => (
                    <TagChip key={t} tag={t} />
                  ))}
                </div>
              </div>
              <Button
                size="sm"
                variant="ghost"
                className="text-muted-foreground hover:text-red-600 h-7 w-7 p-0"
                onClick={() => remove(l.id)}
                title="Delete lesson"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))
        )}
      </CardContent>
    </Card>
  );
}
