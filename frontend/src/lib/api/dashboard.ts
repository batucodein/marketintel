import { apiFetch } from "./client";
import type { OverviewStats, TopLead, AICostSummary } from "../types/dashboard";

export function getOverview(): Promise<OverviewStats> {
  return apiFetch("/dashboard/overview");
}

export function getTopLeads(): Promise<{ leads: TopLead[]; count: number }> {
  return apiFetch("/dashboard/top-leads");
}

export function getAICosts(): Promise<AICostSummary> {
  return apiFetch("/dashboard/ai-costs");
}
