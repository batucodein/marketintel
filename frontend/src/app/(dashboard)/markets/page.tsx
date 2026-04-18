"use client";

import useSWR from "swr";
import { getMarkets } from "@/lib/api/markets";
import type { Market } from "@/lib/types/market";
import { MarketCard } from "@/components/markets/market-card";
import { EmptyState } from "@/components/shared/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { AlertCircle } from "lucide-react";

export default function MarketsPage() {
  const { data: markets, error, mutate } = useSWR<Market[]>(
    "/markets",
    () => getMarkets(),
  );

  if (error) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">Markets</h1>
        <div className="text-center py-12">
          <AlertCircle className="h-8 w-8 text-red-500 mx-auto mb-3" />
          <p className="text-sm text-red-600">{error.message}</p>
        </div>
      </div>
    );
  }

  if (!markets) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-40" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Markets</h1>
        <span className="text-sm text-muted-foreground">{markets.length} markets</span>
      </div>

      {markets.length === 0 ? (
        <EmptyState
          title="No markets yet"
          description="Markets are created when you explore countries during discovery."
          actionLabel="Start Discovery"
          actionHref="/discover"
        />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {markets.map((market) => (
            <MarketCard key={market.id} market={market} onDeleted={() => mutate()} />
          ))}
        </div>
      )}
    </div>
  );
}
