"use client";

import type { ShipmentData } from "@/lib/types/business";
import { CountryFlag } from "@/components/shared/country-flag";
import { toCountryCode, countryName } from "@/lib/utils/country";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatCurrency, formatNumber } from "@/lib/utils/format";
import { Package, DollarSign, Weight, Calendar } from "lucide-react";

interface ImportHistoryProps {
  shipmentData: ShipmentData;
}

export function ImportHistory({ shipmentData }: ImportHistoryProps) {
  return (
    <div className="space-y-4">
      {/* Summary grid */}
      <div className="grid grid-cols-2 gap-3">
        <SummaryCard
          icon={Package}
          label="Total Shipments"
          value={formatNumber(shipmentData.transaction_count)}
        />
        <SummaryCard
          icon={DollarSign}
          label="Total Value"
          value={formatCurrency(shipmentData.total_value_usd)}
        />
        <SummaryCard
          icon={Weight}
          label="Total Weight"
          value={`${formatNumber(shipmentData.total_weight_kg)} kg`}
        />
        <SummaryCard
          icon={Calendar}
          label="Last Shipment"
          value={shipmentData.last_shipment_date || "Unknown"}
        />
      </div>

      {/* Products Imported */}
      {shipmentData.products?.length > 0 && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Products Imported
          </h4>
          <ul className="space-y-1">
            {shipmentData.products.map((product, i) => (
              <li key={i} className="text-sm text-muted-foreground">
                {product}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* HS Codes */}
      {shipmentData.hs_codes?.length > 0 && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            HS Codes
          </h4>
          <div className="flex flex-wrap gap-1.5">
            {shipmentData.hs_codes.map((code) => (
              <Badge key={code} variant="secondary" className="text-[10px] font-mono">
                {code}
              </Badge>
            ))}
          </div>
        </div>
      )}

      {/* Known Suppliers */}
      {shipmentData.suppliers?.length > 0 && (
        <div className="space-y-1.5">
          <h4 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Known Suppliers
          </h4>
          <div className="space-y-2">
            {shipmentData.suppliers.map((supplier, i) => {
              const code = toCountryCode(supplier.country);
              const location = [supplier.city, countryName(code)].filter(Boolean).join(", ");
              return (
                <Card key={i} size="sm">
                  <CardContent className="flex items-center gap-2 py-1">
                    <CountryFlag code={code} showName={false} />
                    <div className="min-w-0">
                      <p className="text-sm font-medium truncate">{supplier.name}</p>
                      <p className="text-xs text-muted-foreground">{location}</p>
                    </div>
                  </CardContent>
                </Card>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

function SummaryCard({
  icon: Icon,
  label,
  value,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
}) {
  return (
    <div className="flex items-center gap-2.5 rounded-lg border bg-muted/30 px-3 py-2">
      <Icon className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="min-w-0">
        <p className="text-[10px] uppercase tracking-wider text-muted-foreground">{label}</p>
        <p className="text-sm font-semibold truncate">{value}</p>
      </div>
    </div>
  );
}
