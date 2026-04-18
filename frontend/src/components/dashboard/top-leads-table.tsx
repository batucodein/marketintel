"use client";

import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { ScoreBar } from "@/components/shared/score-bar";
import { CountryFlag } from "@/components/shared/country-flag";
import type { TopLead } from "@/lib/types/dashboard";

export function TopLeadsTable() {
  const { data, isLoading } = useSWR<{ leads: TopLead[]; count: number }>("/dashboard/top-leads");

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Top Leads</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : !data?.leads?.length ? (
          <p className="text-sm text-muted-foreground text-center py-8">
            No leads scored yet. Start a discovery to find buyers.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Business</TableHead>
                <TableHead>Score</TableHead>
                <TableHead className="hidden md:table-cell">Type</TableHead>
                <TableHead className="hidden md:table-cell">Location</TableHead>
                <TableHead className="hidden lg:table-cell">Market</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.leads.map((lead) => (
                <TableRow key={lead.business_id} className="cursor-pointer hover:bg-accent/50 transition-colors duration-150">
                  <TableCell className="font-medium">{lead.business_name}</TableCell>
                  <TableCell>
                    <ScoreBar value={lead.overall_score} />
                  </TableCell>
                  <TableCell className="hidden md:table-cell text-sm text-muted-foreground">
                    {lead.business_type || "-"}
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    {lead.country_code ? (
                      <CountryFlag code={lead.country_code} showName={false} />
                    ) : (
                      "-"
                    )}
                    {lead.city && <span className="ml-1 text-sm text-muted-foreground">{lead.city}</span>}
                  </TableCell>
                  <TableCell className="hidden lg:table-cell text-sm text-muted-foreground">
                    {lead.market_name}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
