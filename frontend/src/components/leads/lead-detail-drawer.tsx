"use client";

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
import { X, BadgeCheck, MapPin } from "lucide-react";
import { cn } from "@/lib/utils";

interface LeadDetailDrawerProps {
  lead: BusinessWithRelevance | null;
  onClose: () => void;
}

export function LeadDetailDrawer({ lead, onClose }: LeadDetailDrawerProps) {
  if (!lead) return null;

  const isTendataVerified = lead.data_source === "tendata_verified";
  const hasScoring = lead.overall_score != null;
  const shipmentData = lead.social_links?.shipment_data ?? null;

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
            <Button variant="ghost" size="icon" onClick={onClose}>
              <X className="h-4 w-4" />
            </Button>
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
        </div>

        <div className="p-6 space-y-6">
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
