"use client";

import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Globe, Building2, Users, Search } from "lucide-react";
import type { OverviewStats } from "@/lib/types/dashboard";
import { formatNumber } from "@/lib/utils/format";

const STAT_CONFIG = [
  { key: "total_markets" as const, label: "Markets", icon: Globe, color: "text-blue-600" },
  { key: "total_businesses" as const, label: "Businesses", icon: Building2, color: "text-emerald-600" },
  { key: "total_leads" as const, label: "Scored Leads", icon: Users, color: "text-amber-600" },
  { key: "total_searches" as const, label: "Searches", icon: Search, color: "text-purple-600" },
];

export function StatsGrid() {
  const { data, isLoading } = useSWR<OverviewStats>("/dashboard/overview");

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
      {STAT_CONFIG.map((stat) => (
        <Card key={stat.key}>
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {stat.label}
            </CardTitle>
            <stat.icon className={`h-4 w-4 ${stat.color}`} />
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <Skeleton className="h-8 w-20" />
            ) : (
              <p className="text-2xl font-bold font-mono">
                {data ? formatNumber(data[stat.key]) : "0"}
              </p>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
