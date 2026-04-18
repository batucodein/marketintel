"use client";

import useSWR from "swr";
import { getAICosts } from "@/lib/api/dashboard";
import type { AICostSummary } from "@/lib/types/dashboard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Cpu, Calendar, Search, Zap, Coins, MessageSquare, Hash, Loader2 } from "lucide-react";

function formatCost(usd: number): string {
  if (usd < 0.01) return `$${usd.toFixed(6)}`;
  if (usd < 1) return `$${usd.toFixed(4)}`;
  return `$${usd.toFixed(2)}`;
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

export default function CostsPage() {
  const { data: costs, isLoading } = useSWR<AICostSummary>("ai-costs", getAICosts);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">AI Costs</h1>
        <div className="flex items-center gap-2 text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span>Loading cost data...</span>
        </div>
      </div>
    );
  }

  if (!costs) {
    return (
      <div className="space-y-6">
        <h1 className="text-2xl font-bold">AI Costs</h1>
        <p className="text-muted-foreground">No cost data available yet. Run a discovery to start tracking.</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">AI Costs</h1>

      {/* KPI cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-muted-foreground mb-1">
              <Coins className="h-4 w-4" />
              <span className="text-sm">Total Cost</span>
            </div>
            <p className="text-3xl font-bold font-mono">{formatCost(costs.total_cost_usd)}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-muted-foreground mb-1">
              <Zap className="h-4 w-4" />
              <span className="text-sm">AI Calls</span>
            </div>
            <p className="text-3xl font-bold font-mono">{costs.total_requests.toLocaleString()}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-muted-foreground mb-1">
              <MessageSquare className="h-4 w-4" />
              <span className="text-sm">Input Tokens</span>
            </div>
            <p className="text-3xl font-bold font-mono">{formatTokens(costs.total_input_tokens)}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-muted-foreground mb-1">
              <Hash className="h-4 w-4" />
              <span className="text-sm">Output Tokens</span>
            </div>
            <p className="text-3xl font-bold font-mono">{formatTokens(costs.total_output_tokens)}</p>
          </CardContent>
        </Card>
      </div>

      {/* Cost per Discovery */}
      {costs.by_search && costs.by_search.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Search className="h-4 w-4" /> Cost per Discovery
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Upload</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Leads</TableHead>
                  <TableHead className="text-right">AI Calls</TableHead>
                  <TableHead className="text-right">Tokens</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {costs.by_search.map((s) => (
                  <TableRow key={s.search_id}>
                    <TableCell>
                      <div>
                        <span className="text-sm font-medium">{s.source_file_name || "Unknown"}</span>
                        <span className="block text-xs text-muted-foreground">
                          {new Date(s.created_at).toLocaleDateString("en-GB")}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={s.status === "completed" ? "default" : "outline"}
                        className="text-xs"
                      >
                        {s.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {s.result_count ?? "--"}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {s.requests.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {formatTokens(s.input_tokens + s.output_tokens)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs font-medium">
                      {formatCost(s.cost_usd)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {/* Cost by Model */}
      {costs.by_model && costs.by_model.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Cpu className="h-4 w-4" /> Cost by Model
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Provider</TableHead>
                  <TableHead>Model</TableHead>
                  <TableHead className="text-right">Calls</TableHead>
                  <TableHead className="text-right">Input</TableHead>
                  <TableHead className="text-right">Output</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {costs.by_model.map((m) => (
                  <TableRow key={`${m.provider}-${m.model}`}>
                    <TableCell className="text-xs">{m.provider}</TableCell>
                    <TableCell className="font-mono text-xs">{m.model}</TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {m.requests.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {formatTokens(m.input_tokens)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {formatTokens(m.output_tokens)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs font-medium">
                      {formatCost(m.cost_usd)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {/* Daily Costs */}
      {costs.by_day && costs.by_day.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Calendar className="h-4 w-4" /> Daily Costs (Last 30 Days)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Date</TableHead>
                  <TableHead className="text-right">AI Calls</TableHead>
                  <TableHead className="text-right">Tokens</TableHead>
                  <TableHead className="text-right">Cost</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {costs.by_day.map((d) => (
                  <TableRow key={d.date}>
                    <TableCell className="text-sm">{d.date}</TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {d.requests.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs">
                      {formatTokens(d.input_tokens + d.output_tokens)}
                    </TableCell>
                    <TableCell className="text-right font-mono text-xs font-medium">
                      {formatCost(d.cost_usd)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {costs.total_requests === 0 && (
        <Card>
          <CardContent className="py-12 text-center">
            <p className="text-muted-foreground">No AI costs tracked yet. Run a discovery to start tracking costs.</p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
