export interface OverviewStats {
  total_markets: number;
  total_businesses: number;
  total_leads: number;
  total_searches: number;
}

export interface TopLead {
  business_id: string;
  business_name: string;
  overall_score: number;
  business_type: string | null;
  city: string | null;
  country_code: string | null;
  market_name: string;
}

export interface AICostSummary {
  total_cost_usd: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_requests: number;
  by_search: SearchCostSummary[];
  by_day: DailyCostSummary[];
  by_model: ModelCostSummary[];
}

export interface SearchCostSummary {
  search_id: string;
  source_file_name: string;
  status: string;
  result_count: number | null;
  cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  requests: number;
  created_at: string;
}

export interface DailyCostSummary {
  date: string;
  cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  requests: number;
}

export interface ModelCostSummary {
  provider: string;
  model: string;
  cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  requests: number;
}
