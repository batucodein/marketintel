export interface MarketSummary {
  market_id: string;
  destination_country: string;
  dominant_hs_code: string;
  origin_country: string;
  origin_share: number;
  importer_count: number;
  shipment_count: number;
}

export interface UploadResult {
  search_id: string;
  status: string;
  markets: MarketSummary[];
  total_importers: number;
  total_shipments: number;
  warnings?: string[];
}

export interface SearchState {
  market_ids?: string[];
  source_file_name?: string;
  uploaded_at?: string;
  warnings?: string[];
}

export interface Search {
  id: string;
  user_id: string;
  search_type: string;
  query: SearchState;
  status: "processing" | "enriching" | "scoring" | "completed" | "failed";
  result_count: number | null;
  completed_at: string | null;
  task_id: string | null;
  created_at: string;
  updated_at: string;
}
