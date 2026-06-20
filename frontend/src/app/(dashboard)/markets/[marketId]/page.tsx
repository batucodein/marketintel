"use client";

import { useParams } from "next/navigation";
import Link from "next/link";
import useSWR from "swr";
import { getMarket, assignBrandToMarket } from "@/lib/api/markets";
import { listSenderProfiles } from "@/lib/api/outreach";
import type { Market } from "@/lib/types/market";
import type { SenderProfile } from "@/lib/types/outreach";
import { CountryFlag } from "@/components/shared/country-flag";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { buttonVariants } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AlertCircle,
  ArrowRight,
  Calendar,
  Package,
  Users,
  FileSpreadsheet,
} from "lucide-react";

export default function MarketDetailPage() {
  const { marketId } = useParams<{ marketId: string }>();

  const { data: market, error, mutate } = useSWR<Market>(
    `/markets/${marketId}`,
    () => getMarket(marketId),
  );

  if (error) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">Market Detail</h1>
        <div className="text-center py-12">
          <AlertCircle className="h-8 w-8 text-red-500 mx-auto mb-3" />
          <p className="text-sm text-red-600">{error.message}</p>
        </div>
      </div>
    );
  }

  if (!market) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
        </div>
      </div>
    );
  }

  const title = market.derived_product_name ?? market.name;

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold">{title}</h1>
          <div className="flex items-center gap-3 mt-1 flex-wrap">
            {market.dominant_hs_code && (
              <span className="text-sm font-mono text-muted-foreground">
                HS {market.dominant_hs_code}
              </span>
            )}
            {market.origin_country && (
              <>
                <span className="text-sm text-muted-foreground">·</span>
                <span className="text-sm flex items-center gap-1">
                  <CountryFlag code={market.origin_country} />
                  {market.origin_country}
                  <ArrowRight className="h-3 w-3 mx-1 text-muted-foreground" />
                  <CountryFlag code={market.country_code} />
                  {market.country_code}
                </span>
              </>
            )}
          </div>
        </div>
        <Link
          href={`/markets/${marketId}/leads`}
          className={buttonVariants({ variant: "default" })}
        >
          View Leads
          <ArrowRight className="h-4 w-4 ml-1" />
        </Link>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        {market.importer_count != null && (
          <StatCard
            label="Importers"
            value={market.importer_count.toLocaleString()}
            icon={<Users className="h-4 w-4" />}
          />
        )}
        {market.shipment_count != null && (
          <StatCard
            label="Shipments"
            value={market.shipment_count.toLocaleString()}
            icon={<Package className="h-4 w-4" />}
          />
        )}
        {market.shipment_from_date && market.shipment_to_date && (
          <StatCard
            label="Date range"
            value={formatRangeShort(market.shipment_from_date, market.shipment_to_date)}
            icon={<Calendar className="h-4 w-4" />}
          />
        )}
        {market.source_file_name && (
          <StatCard
            label="Source file"
            value={market.source_file_name}
            icon={<FileSpreadsheet className="h-4 w-4" />}
            tooltip={market.source_file_name}
          />
        )}
      </div>

      <BrandCard
        marketId={marketId}
        senderProfileId={market.sender_profile_id}
        onChange={() => mutate()}
      />

      {market.all_hs_codes && market.all_hs_codes.length > 1 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Related HS codes in this data</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-2">
              {market.all_hs_codes.map((code) => (
                <span
                  key={code}
                  className="text-xs font-mono px-2 py-1 bg-muted rounded"
                >
                  {code}
                </span>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {market.origin_countries && market.origin_countries.length > 1 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">
              Origin countries ({market.origin_countries.length})
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-2">
              {market.origin_countries.map((code) => (
                <span key={code} className="text-xs flex items-center gap-1 px-2 py-1 bg-muted rounded">
                  <CountryFlag code={code} />
                  {code}
                </span>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

// BrandCard lets the user assign the market's fixed brand (sender profile).
// Email Groups created from this market inherit it.
function BrandCard({
  marketId,
  senderProfileId,
  onChange,
}: {
  marketId: string;
  senderProfileId: string | null;
  onChange: () => void;
}) {
  const { data: profiles } = useSWR<SenderProfile[]>(
    "/outreach/sender-profile",
    () => listSenderProfiles(),
  );

  async function handleChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const value = e.target.value || null;
    try {
      await assignBrandToMarket(marketId, value);
      onChange();
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to assign brand");
    }
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Sender brand</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="text-xs text-muted-foreground">
          The brand this market sends as. Email Groups created from this market use it.
          {!senderProfileId && " Assign one before creating a group."}
        </p>
        <select
          value={senderProfileId ?? ""}
          onChange={handleChange}
          className="w-full md:w-80 h-9 rounded-md border border-input bg-background px-3 text-sm"
        >
          <option value="">— No brand assigned —</option>
          {(profiles ?? []).map((p) => (
            <option key={p.id} value={p.id}>
              {p.name || p.company_name}
            </option>
          ))}
        </select>
        {(profiles ?? []).length === 0 && (
          <p className="text-xs text-muted-foreground">
            No brands yet —{" "}
            <Link href="/outreach/settings/profile" className="underline">
              create one
            </Link>
            .
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function StatCard({
  label,
  value,
  icon,
  tooltip,
}: {
  label: string;
  value: string;
  icon: React.ReactNode;
  tooltip?: string;
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-2">
          {icon}
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-sm font-mono truncate" title={tooltip}>
          {value}
        </p>
      </CardContent>
    </Card>
  );
}

function formatRangeShort(from: string, to: string): string {
  const fmt = (iso: string) =>
    new Date(iso).toLocaleDateString("en-GB", { month: "short", year: "numeric" });
  return `${fmt(from)} – ${fmt(to)}`;
}
