import { apiFetch } from "./client";
import type { Market } from "../types/market";
import type { BusinessWithRelevance } from "../types/business";

export function getMarkets(): Promise<Market[]> {
  return apiFetch("/markets");
}

export function getMarket(marketId: string): Promise<Market> {
  return apiFetch(`/markets/${marketId}`);
}

export function deleteMarket(marketId: string): Promise<void> {
  return apiFetch(`/markets/${marketId}`, { method: "DELETE" });
}

export function getMarketLeads(
  marketId: string,
  page = 1,
  pageSize = 20,
  minScore = 0,
): Promise<{
  market_id: string;
  leads: BusinessWithRelevance[];
  total: number;
  page: number;
  page_size: number;
}> {
  return apiFetch(
    `/markets/${marketId}/leads?page=${page}&page_size=${pageSize}&min_score=${minScore}`,
  );
}
