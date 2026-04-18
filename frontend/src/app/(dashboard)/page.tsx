"use client";

import Link from "next/link";
import { buttonVariants } from "@/components/ui/button";
import { StatsGrid } from "@/components/dashboard/stats-grid";
import { TopLeadsTable } from "@/components/dashboard/top-leads-table";
import { Search } from "lucide-react";

export default function DashboardPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <Link href="/discover" className={buttonVariants({ className: "cursor-pointer" })}>
          <Search className="mr-2 h-4 w-4" />
          New Discovery
        </Link>
      </div>
      <StatsGrid />
      <TopLeadsTable />
    </div>
  );
}
