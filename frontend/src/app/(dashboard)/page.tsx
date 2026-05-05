"use client";

import Link from "next/link";
import useSWR from "swr";
import { buttonVariants } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { StatsGrid } from "@/components/dashboard/stats-grid";
import { TopLeadsTable } from "@/components/dashboard/top-leads-table";
import { tasksOverdueCount } from "@/lib/api/outreach";
import { Search, AlertCircle } from "lucide-react";

export default function DashboardPage() {
  const { data: overdue } = useSWR("/outreach/tasks/overdue-count", () => tasksOverdueCount());

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <Link href="/discover" className={buttonVariants({ className: "cursor-pointer" })}>
          <Search className="mr-2 h-4 w-4" />
          New Discovery
        </Link>
      </div>

      {overdue && overdue.overdue > 0 && (
        <Link href="/outreach/tasks">
          <Card className="bg-amber-50 border-amber-200 hover:bg-amber-100 transition-colors cursor-pointer">
            <CardContent className="py-3 flex items-center gap-3">
              <AlertCircle className="h-5 w-5 text-amber-700" />
              <div className="flex-1">
                <p className="text-sm font-medium">
                  {overdue.overdue} overdue task{overdue.overdue === 1 ? "" : "s"}
                </p>
                <p className="text-xs text-muted-foreground">
                  Review them in the outreach tasks page.
                </p>
              </div>
            </CardContent>
          </Card>
        </Link>
      )}

      <StatsGrid />
      <TopLeadsTable />
    </div>
  );
}
