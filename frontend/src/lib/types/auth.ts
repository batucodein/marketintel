export interface User {
  id: string;
  email: string;
  company_name: string | null;
  home_country: string | null;
  subscription_tier: string;
  api_calls_remaining: number;
  created_at: string;
  updated_at: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  token_type: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  company_name?: string;
  home_country?: string;
}
