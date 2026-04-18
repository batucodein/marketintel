"use client";

import type { BusinessWithRelevance } from "@/lib/types/business";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { TrustTierBadge } from "@/components/leads/trust-tier-badge";
import { ScoreBar } from "@/components/shared/score-bar";
import { CountryFlag } from "@/components/shared/country-flag";
import {
  ChevronRight,
  Globe,
  BadgeCheck,
  FileSpreadsheet,
  MapPin,
  Database,
  Phone,
  Mail,
} from "lucide-react";

function SourceBadge({ source }: { source: string | null | undefined }) {
  if (!source) {
    return <span className="text-xs text-muted-foreground">--</span>;
  }

  switch (source) {
    case "tendata_verified":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-green-700">
          <BadgeCheck className="h-3.5 w-3.5" />
          Verified
        </span>
      );
    case "tendata_import":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-amber-700">
          <FileSpreadsheet className="h-3.5 w-3.5" />
          Trade Data
        </span>
      );
    case "google_places":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-blue-700">
          <MapPin className="h-3.5 w-3.5" />
          Google
        </span>
      );
    default:
      return (
        <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
          <Database className="h-3.5 w-3.5" />
          {source}
        </span>
      );
  }
}

function OverallScoreDot({ score }: { score: number }) {
  const color =
    score > 75 ? "bg-green-500" : score > 50 ? "bg-amber-500" : "bg-red-500";
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium">
      <span className={`inline-block h-2 w-2 rounded-full ${color}`} />
      {Math.round(score)}
    </span>
  );
}

interface LeadTableProps {
  leads: BusinessWithRelevance[];
  onSelectLead: (lead: BusinessWithRelevance) => void;
  page?: number;
  pageSize?: number;
}

export function LeadTable({ leads, onSelectLead, page = 1, pageSize = 20 }: LeadTableProps) {
  const startIndex = (page - 1) * pageSize;
  return (
    <div className="rounded-lg border bg-card overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">#</TableHead>
            <TableHead>Company</TableHead>
            <TableHead className="w-12" />
            <TableHead>Trust Tier</TableHead>
            <TableHead className="hidden md:table-cell">Score</TableHead>
            <TableHead className="hidden md:table-cell">Relevance</TableHead>
            <TableHead className="hidden md:table-cell">Location</TableHead>
            <TableHead className="hidden lg:table-cell">Industry</TableHead>
            <TableHead className="hidden lg:table-cell">Source</TableHead>
            <TableHead className="w-10" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {leads.map((lead, i) => (
            <TableRow
              key={lead.id}
              className="cursor-pointer hover:bg-blue-50/50 transition-colors"
              onClick={() => onSelectLead(lead)}
            >
              <TableCell className="text-muted-foreground font-mono text-xs">{startIndex + i + 1}</TableCell>
              <TableCell>
                <span className="font-medium text-sm">{lead.name}</span>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-1">
                  {lead.website && (
                    <Globe className="h-3.5 w-3.5 text-muted-foreground" />
                  )}
                  {lead.phone && (
                    <Phone className="h-3.5 w-3.5 text-muted-foreground" />
                  )}
                  {lead.email && (
                    <Mail className="h-3.5 w-3.5 text-muted-foreground" />
                  )}
                </div>
              </TableCell>
              <TableCell>
                <TrustTierBadge tier={lead.trust_tier} />
              </TableCell>
              <TableCell className="hidden md:table-cell">
                {lead.overall_score != null ? (
                  <OverallScoreDot score={lead.overall_score} />
                ) : (
                  <span className="text-xs text-muted-foreground">--</span>
                )}
              </TableCell>
              <TableCell className="hidden md:table-cell">
                {lead.relevance_score != null ? (
                  <ScoreBar value={lead.relevance_score * 100} />
                ) : (
                  <span className="text-xs text-muted-foreground">--</span>
                )}
              </TableCell>
              <TableCell className="hidden md:table-cell">
                <div className="flex items-center gap-1.5">
                  {lead.country_code && <CountryFlag code={lead.country_code} />}
                  {lead.city && (
                    <span className="text-xs text-muted-foreground">{lead.city}</span>
                  )}
                </div>
              </TableCell>
              <TableCell className="hidden lg:table-cell">
                {lead.industry ? (
                  <span className="text-xs">{lead.industry}</span>
                ) : (
                  <span className="text-xs text-muted-foreground">--</span>
                )}
              </TableCell>
              <TableCell className="hidden lg:table-cell">
                <SourceBadge source={lead.discovered_via || lead.data_source} />
              </TableCell>
              <TableCell>
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
