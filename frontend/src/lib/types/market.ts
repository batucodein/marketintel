export interface Market {
  id: string;
  name: string;
  country_code: string;
  region: string | null;
  city: string | null;
  latitude: number | null;
  longitude: number | null;
  radius_km: number | null;
  product_category_id: string | null;
  estimated_market_size: number | null;
  saturation_score: number | null;
  demand_score: number | null;
  last_analyzed_at: string | null;

  // Derived from Excel upload
  derived_product_name: string | null;
  dominant_hs_code: string | null;
  all_hs_codes: string[] | null;
  origin_country: string | null;
  origin_countries: string[] | null;
  origin_share: number | null;
  shipment_from_date: string | null;
  shipment_to_date: string | null;
  importer_count: number | null;
  shipment_count: number | null;
  source_file_name: string | null;
  uploaded_at: string | null;

  created_at: string;
  updated_at: string;
}
