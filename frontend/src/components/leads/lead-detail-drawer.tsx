"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import type { BusinessWithRelevance } from "@/lib/types/business";
import { TrustTierBadge } from "@/components/leads/trust-tier-badge";
import { ScoringRationale } from "@/components/leads/scoring-rationale";
import { ScoreBreakdown } from "@/components/leads/score-breakdown";
import { ImportHistory } from "@/components/leads/import-history";
import { GooglePlacesProfile } from "@/components/leads/google-places-profile";
import { ScoreBar } from "@/components/shared/score-bar";
import { CountryFlag } from "@/components/shared/country-flag";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { X, BadgeCheck, MapPin, Mail, Loader2, Pencil, Save, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import { apiFetch } from "@/lib/api/client";
import { startConversation } from "@/lib/api/outreach";
import { updateLead } from "@/lib/api/markets";

interface LeadDetailDrawerProps {
  lead: BusinessWithRelevance | null;
  marketId?: string;
  onClose: () => void;
  onLeadUpdated?: (updated: BusinessWithRelevance) => void;
}

export function LeadDetailDrawer({ lead, marketId, onClose, onLeadUpdated }: LeadDetailDrawerProps) {
  const router = useRouter();
  const [emailing, setEmailing] = useState(false);
  const [emailError, setEmailError] = useState<string | null>(null);

  // Inline edit state for contact info.
  const [editing, setEditing] = useState(false);
  const [editEmail, setEditEmail] = useState("");
  const [editPhone, setEditPhone] = useState("");
  const [editWebsite, setEditWebsite] = useState("");
  const [savingContact, setSavingContact] = useState(false);
  const [contactError, setContactError] = useState<string | null>(null);

  function startEdit() {
    if (!lead) return;
    setEditEmail(lead.email ?? "");
    setEditPhone(lead.phone ?? "");
    setEditWebsite(lead.website ?? "");
    setEditing(true);
    setContactError(null);
  }

  async function saveContact() {
    if (!lead || !marketId) return;
    setSavingContact(true);
    setContactError(null);
    try {
      await updateLead(marketId, lead.id, {
        email: editEmail,
        phone: editPhone,
        website: editWebsite,
      });
      const updated = { ...lead, email: editEmail || null, phone: editPhone || null, website: editWebsite || null };
      onLeadUpdated?.(updated as BusinessWithRelevance);
      setEditing(false);
    } catch (e) {
      setContactError(e instanceof Error ? e.message : "Failed to save");
    } finally {
      setSavingContact(false);
    }
  }

  if (!lead) return null;

  const isTendataVerified = lead.data_source === "tendata_verified";
  const hasScoring = lead.overall_score != null;
  const shipmentData = lead.social_links?.shipment_data ?? null;

  async function handleEmail() {
    if (!lead) return;
    setEmailing(true);
    setEmailError(null);
    try {
      // 1) Upsert contact for this business (backend handler: ensure-contact or similar).
      //    We call a small endpoint that returns a contact_id for the business.
      const contact = await apiFetch<{ id: string }>(
        `/outreach/contacts/ensure?business_id=${lead.id}`,
        { method: "POST" },
      ).catch(async () => {
        // Fallback: the endpoint doesn't exist yet. Try listing contacts and matching.
        throw new Error("contact_upsert_endpoint_missing");
      });
      const res = await startConversation(contact.id, true);
      router.push(`/outreach/${res.conversation.id}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to start conversation";
      setEmailError(msg);
      setEmailing(false);
    }
  }

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-40 bg-black/30 animate-in fade-in"
        onClick={onClose}
      />

      {/* Drawer */}
      <div className="fixed inset-y-0 right-0 z-50 w-full max-w-md bg-background border-l shadow-xl animate-in slide-in-from-right overflow-y-auto">
        {/* Header */}
        <div className="sticky top-0 z-10 border-b bg-background px-6 py-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-bold truncate pr-4">{lead.name}</h2>
            <div className="flex items-center gap-1">
              <Button
                size="sm"
                onClick={handleEmail}
                disabled={emailing || !lead.email}
                title={lead.email ?? "No email on file for this lead"}
              >
                {emailing ? (
                  <Loader2 className="h-4 w-4 animate-spin mr-1" />
                ) : (
                  <Mail className="h-4 w-4 mr-1" />
                )}
                Email
              </Button>
              <Button variant="ghost" size="icon" onClick={onClose}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
          <div className="flex items-center gap-3 mt-2">
            <TrustTierBadge tier={lead.trust_tier} />
            {lead.relevance_score != null && (
              <div className="flex items-center gap-2 flex-1">
                <span className="text-xs text-muted-foreground">Relevance</span>
                <ScoreBar value={lead.relevance_score * 100} />
              </div>
            )}
            {isTendataVerified && (
              <BadgeCheck className="h-4 w-4 text-emerald-600 shrink-0" />
            )}
          </div>
          {emailError && (
            <div className="mt-2 text-xs text-red-600">{emailError}</div>
          )}
        </div>

        <div className="p-6 space-y-6">
          {/* Contact info — editable */}
          <Section title="Contact Info">
            {!editing ? (
              <div className="space-y-1 text-sm">
                <div className="grid grid-cols-[80px_1fr_auto] items-center gap-2">
                  <span className="text-xs text-muted-foreground">Email</span>
                  <span className="font-mono truncate">{lead.email ?? <em className="text-muted-foreground">not set</em>}</span>
                  {marketId && (
                    <Button variant="ghost" size="icon" onClick={startEdit} title="Edit contact info">
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                  )}
                </div>
                <div className="grid grid-cols-[80px_1fr_auto] items-center gap-2">
                  <span className="text-xs text-muted-foreground">Phone</span>
                  <span className="font-mono truncate">{lead.phone ?? <em className="text-muted-foreground">not set</em>}</span>
                  <span></span>
                </div>
                <div className="grid grid-cols-[80px_1fr_auto] items-center gap-2">
                  <span className="text-xs text-muted-foreground">Website</span>
                  <span className="font-mono truncate">{lead.website ?? <em className="text-muted-foreground">not set</em>}</span>
                  <span></span>
                </div>
              </div>
            ) : (
              <div className="space-y-2 text-sm">
                <div>
                  <label className="text-xs text-muted-foreground block mb-0.5">Email</label>
                  <Input
                    value={editEmail}
                    onChange={(e) => setEditEmail(e.target.value)}
                    placeholder="name@company.com"
                    type="email"
                  />
                </div>
                <div>
                  <label className="text-xs text-muted-foreground block mb-0.5">Phone</label>
                  <Input
                    value={editPhone}
                    onChange={(e) => setEditPhone(e.target.value)}
                    placeholder="+1 555 123 4567"
                  />
                </div>
                <div>
                  <label className="text-xs text-muted-foreground block mb-0.5">Website</label>
                  <Input
                    value={editWebsite}
                    onChange={(e) => setEditWebsite(e.target.value)}
                    placeholder="https://example.com"
                  />
                </div>
                {contactError && <p className="text-xs text-red-600">{contactError}</p>}
                <div className="flex gap-2 pt-1">
                  <Button size="sm" onClick={saveContact} disabled={savingContact}>
                    {savingContact ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : <Save className="h-3.5 w-3.5 mr-1" />}
                    Save
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setEditing(false)} disabled={savingContact}>
                    <XCircle className="h-3.5 w-3.5 mr-1" />
                    Cancel
                  </Button>
                </div>
              </div>
            )}
          </Section>

          {/* AI Scoring */}
          {hasScoring && (
            <Section title="AI Scoring">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Overall Score
                </span>
                <span className="text-lg font-bold font-mono">{lead.overall_score}</span>
              </div>
              <ScoreBreakdown lead={lead} />
              <ScoringRationale lead={lead} />
            </Section>
          )}

          {/* Import History */}
          {shipmentData && (
            <Section title="Import History">
              <ImportHistory shipmentData={shipmentData} />
            </Section>
          )}

          {/* Company Profile (Google Places) */}
          <Section title="Company Profile">
            <GooglePlacesProfile business={lead} />
          </Section>

          {/* Location */}
          <Section title="Location">
            <div className="flex items-start gap-2">
              <MapPin className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
              <div className="text-sm">
                {lead.country_code && (
                  <CountryFlag code={lead.country_code} />
                )}
                {lead.city && <span className="ml-1">{lead.city}</span>}
                {lead.address && (
                  <p className="text-xs text-muted-foreground mt-0.5">
                    {lead.address}
                  </p>
                )}
              </div>
            </div>
          </Section>

          {/* Data Footer */}
          <Section title="Data">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Source</span>
              <DataSourceBadge source={lead.data_source} />
            </div>
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Enrichment</span>
              <span
                className={cn(
                  "text-xs",
                  lead.enrichment_status === "enriched"
                    ? "text-emerald-600"
                    : "text-muted-foreground"
                )}
              >
                {lead.enrichment_status}
              </span>
            </div>
            {lead.data_confidence != null && (
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted-foreground">Data Completeness</span>
                <span className="font-mono text-xs">
                  {(lead.data_confidence * 100).toFixed(0)}%
                </span>
              </div>
            )}
          </Section>
        </div>
      </div>
    </>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        {title}
      </h3>
      {children}
    </div>
  );
}

function DataSourceBadge({ source }: { source: string }) {
  const config = DATA_SOURCE_CONFIG[source] ?? {
    label: source,
    className: "bg-slate-100 text-slate-700",
  };
  return (
    <Badge className={cn("text-[10px]", config.className)}>
      {config.label}
    </Badge>
  );
}

const DATA_SOURCE_CONFIG: Record<
  string,
  { label: string; className: string }
> = {
  tendata_verified: {
    label: "Tendata + Google Verified",
    className: "bg-emerald-100 text-emerald-800",
  },
  tendata_import: {
    label: "Tendata Import",
    className: "bg-amber-100 text-amber-800",
  },
  google_places: {
    label: "Google Places",
    className: "bg-blue-100 text-blue-800",
  },
};
