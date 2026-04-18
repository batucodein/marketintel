"use client";

import Link from "next/link";
import type { Market } from "@/lib/types/market";
import { deleteMarket } from "@/lib/api/markets";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { CountryFlag } from "@/components/shared/country-flag";
import { Button } from "@/components/ui/button";
import {
  ChevronRight,
  Calendar,
  Users,
  Package,
  ArrowRight,
  Trash2,
} from "lucide-react";

interface MarketCardProps {
  market: Market;
  onDeleted?: () => void;
}

export function MarketCard({ market, onDeleted }: MarketCardProps) {
  const handleDelete = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const label = market.derived_product_name ?? market.name;
    if (!confirm(`Delete "${label}"? This removes all its leads.`)) return;
    await deleteMarket(market.id);
    onDeleted?.();
  };

  const title = market.derived_product_name ?? market.name;
  const hsCode = market.dominant_hs_code;
  const relatedCount = (market.all_hs_codes?.length ?? 1) - 1;
  const origin = market.origin_country;
  const destination = market.country_code;
  const originShareLow =
    market.origin_share != null && market.origin_share < 0.85;
  const otherOrigins = (market.origin_countries?.length ?? 1) - 1;

  const dateRange = formatDateRange(
    market.shipment_from_date,
    market.shipment_to_date,
  );

  return (
    <Link href={`/markets/${market.id}`}>
      <Card className="hover:border-blue-300 hover:shadow-sm transition-all duration-200 cursor-pointer h-full">
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between gap-2">
            <CardTitle className="text-sm leading-snug flex-1">{title}</CardTitle>
            <div className="flex items-center gap-1 shrink-0">
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6 text-muted-foreground hover:text-red-600"
                onClick={handleDelete}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            </div>
          </div>
          {hsCode && (
            <p className="text-xs text-muted-foreground font-mono pt-0.5">
              HS {hsCode}
              {relatedCount > 0 && (
                <span className="ml-2 text-[10px] bg-muted px-1.5 py-0.5 rounded">
                  +{relatedCount} related
                </span>
              )}
            </p>
          )}
        </CardHeader>

        <CardContent className="space-y-2">
          {(origin || destination) && (
            <div className="flex items-center gap-1.5 text-xs">
              {origin && <CountryFlag code={origin} />}
              {origin && <span>{origin}</span>}
              <ArrowRight className="h-3 w-3 text-muted-foreground" />
              <CountryFlag code={destination} />
              <span>{destination}</span>
              {originShareLow && otherOrigins > 0 && (
                <span className="ml-1 text-[10px] text-amber-600 bg-amber-50 px-1.5 py-0.5 rounded">
                  +{otherOrigins} origins
                </span>
              )}
            </div>
          )}

          {dateRange && (
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Calendar className="h-3 w-3" />
              <span>{dateRange}</span>
            </div>
          )}

          <div className="flex items-center gap-3 text-xs text-muted-foreground pt-1">
            {market.importer_count != null && (
              <span className="flex items-center gap-1">
                <Users className="h-3 w-3" />
                {market.importer_count} importers
              </span>
            )}
            {market.shipment_count != null && (
              <span className="flex items-center gap-1">
                <Package className="h-3 w-3" />
                {market.shipment_count} shipments
              </span>
            )}
          </div>

          {market.uploaded_at && (
            <p className="text-[10px] text-muted-foreground pt-1">
              Uploaded{" "}
              {new Date(market.uploaded_at).toLocaleDateString("en-GB", {
                year: "numeric",
                month: "short",
                day: "numeric",
              })}
            </p>
          )}
        </CardContent>
      </Card>
    </Link>
  );
}

function formatDateRange(
  from: string | null,
  to: string | null,
): string | null {
  if (!from || !to) return null;
  const fromDate = new Date(from);
  const toDate = new Date(to);
  const fmt = (d: Date) =>
    d.toLocaleDateString("en-GB", { month: "short", year: "numeric" });

  const months = monthsDiff(fromDate, toDate);
  const rangeText = `${fmt(fromDate)} – ${fmt(toDate)}`;
  if (months > 0) {
    return `${rangeText} (${months} month${months !== 1 ? "s" : ""})`;
  }
  return rangeText;
}

function monthsDiff(from: Date, to: Date): number {
  return (
    (to.getFullYear() - from.getFullYear()) * 12 +
    (to.getMonth() - from.getMonth()) +
    1
  );
}
