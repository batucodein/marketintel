"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import useSWR from "swr";
import { getMarketLeads, getMarket } from "@/lib/api/markets";
import type { Market } from "@/lib/types/market";
import type { BusinessWithRelevance } from "@/lib/types/business";
import { LeadTable } from "@/components/leads/lead-table";
import { LeadDetailDrawer } from "@/components/leads/lead-detail-drawer";
import { PaginationControls } from "@/components/shared/pagination-controls";
import { EmptyState } from "@/components/shared/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { AlertCircle } from "lucide-react";

const PAGE_SIZE = 20;

export default function MarketLeadsPage() {
  const { marketId } = useParams<{ marketId: string }>();
  const [page, setPage] = useState(1);
  const [minScore, setMinScore] = useState(0);
  const [selectedLead, setSelectedLead] = useState<BusinessWithRelevance | null>(null);

  const { data: market } = useSWR<Market>(
    `/markets/${marketId}`,
    () => getMarket(marketId),
  );

  const { data, error } = useSWR(
    `/markets/${marketId}/leads?page=${page}&page_size=${PAGE_SIZE}&min_score=${minScore}`,
    () => getMarketLeads(marketId, page, PAGE_SIZE, minScore),
  );

  if (error) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">Market Leads</h1>
        <div className="text-center py-12">
          <AlertCircle className="h-8 w-8 text-red-500 mx-auto mb-3" />
          <p className="text-sm text-red-600">{error.message}</p>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold">Market Leads</h1>
          {market && (
            <p className="text-sm text-muted-foreground mt-1">
              {market.name} &middot; {data.total} leads
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          <label className="text-xs text-muted-foreground whitespace-nowrap">Min Score</label>
          <Input
            type="number"
            min={0}
            max={100}
            value={minScore}
            onChange={(e) => {
              setMinScore(Number(e.target.value));
              setPage(1);
            }}
            className="w-20 h-8 text-sm"
          />
        </div>
      </div>

      {!data.leads || data.leads.length === 0 ? (
        <EmptyState
          title="No leads found"
          description={minScore > 0 ? `No leads with score above ${minScore}. Try lowering the threshold.` : "No leads in this market yet."}
        />
      ) : (
        <>
          <LeadTable
            leads={data.leads}
            onSelectLead={setSelectedLead}
            page={data.page}
            pageSize={PAGE_SIZE}
          />
          <PaginationControls
            page={data.page}
            pageSize={data.page_size}
            total={data.total}
            onPageChange={setPage}
          />
        </>
      )}

      <LeadDetailDrawer
        lead={selectedLead}
        onClose={() => setSelectedLead(null)}
      />
    </div>
  );
}
