export type TrustTier =
  | "verified_buyer"
  | "verified_importer"
  | "verified"
  | "confirmed_buyer"
  | "confirmed_importer"
  | "potential"
  | "inferred";

export interface SupplierInfo {
  name: string;
  country: string;
  city: string;
}

export interface ShipmentData {
  transaction_count: number;
  total_weight_kg: number;
  total_value_usd: number;
  last_shipment_date: string;
  hs_codes: string[];
  products: string[];
  suppliers: SupplierInfo[];
  buys_from_home: boolean;
  trust_tier: string;
}

export interface GooglePlacesData {
  google_types: string[];
  primary_type: string;
  rating: number;
  rating_count: number;
}

export interface SocialLinksData {
  shipment_data?: ShipmentData;
  google_places_data?: GooglePlacesData;
}

export interface Business {
  id: string;
  name: string;
  website: string | null;
  google_place_id: string | null;
  linkedin_url: string | null;
  country_code: string | null;
  city: string | null;
  address: string | null;
  latitude: number | null;
  longitude: number | null;
  industry: string | null;
  sub_industry: string | null;
  employee_count_range: string | null;
  estimated_revenue_range: string | null;
  business_type: string | null;
  description: string | null;
  phone: string | null;
  email: string | null;
  rating: number | null;
  rating_count: number | null;
  google_types: string[] | null;
  opening_hours: Record<string, string> | null;
  social_links: Record<string, unknown> | null;
  data_source: string;
  enrichment_status: string;
  last_enriched_at: string | null;
  data_confidence: number | null;
  created_at: string;
  updated_at: string;
}

export interface BusinessWithRelevance extends Omit<Business, "social_links"> {
  relevance_score: number | null;
  discovered_via: string | null;
  trust_tier: TrustTier;
  social_links: SocialLinksData | null;
  rating: number | null;
  rating_count: number | null;
  google_types: string[] | null;

  // Scoring fields (present after lead scoring, null before)
  overall_score?: number | null;
  purchase_likelihood?: number | null;
  deal_size_potential?: number | null;
  urgency_score?: number | null;
  fit_score?: number | null;
  accessibility_score?: number | null;
  // Per-dimension data quality, 0..1 each. Present once Phase B scoring
  // has run. Keys: deal_size, purchase_likelihood, accessibility, fit, urgency.
  dimension_completeness?: Record<string, number>;
  // Canonical-field keys absent in input AND not filled by enrichment.
  missing_fields?: string[];
  scoring_rationale?: string | null;
  strengths?: string[];
  weaknesses?: string[];
  recommended_approach?: string | null;
}
