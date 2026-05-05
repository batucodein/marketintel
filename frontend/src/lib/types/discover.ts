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

// --- Dynamic ingestion mapping --------------------------------------

export interface CanonicalField {
  key: string;
  label: string;
  group: string;
  required: boolean;
  affects_dimension?: string;
  description?: string;
}

export interface ColumnMappingPreview {
  sample: {
    headers: string[];
    samples: string[][];
  };
  // header → canonical_key (only confident AI suggestions, can be empty)
  mapping: Record<string, string>;
  // header → 0..1 confidence
  confidence: Record<string, number>;
  // headers the AI didn't map
  unmapped_headers: string[];
  // canonical keys that are required but not yet mapped
  missing_required: string[];
  // header → AI's reason for the mapping (for the UX tooltip)
  reasoning: Record<string, string>;
  canonical: CanonicalField[];
  data_completeness: number;
}
