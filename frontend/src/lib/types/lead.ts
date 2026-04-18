export interface LeadScore {
  id: string;
  business_id: string;
  market_id: string;
  user_id: string;
  overall_score: number;
  purchase_likelihood: number | null;
  deal_size_potential: number | null;
  urgency_score: number | null;
  fit_score: number | null;
  accessibility_score: number | null;
  scoring_rationale: string | null;
  strengths: string[];
  weaknesses: string[];
  recommended_approach: string | null;
  model_version: string;
  prompt_version: string;
  scored_at: string;
  expires_at: string | null;
}
