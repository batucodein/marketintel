import { apiFetch } from "./client";
import type { TokenPair, User, LoginRequest, RegisterRequest } from "../types/auth";

export function login(data: LoginRequest): Promise<TokenPair> {
  return apiFetch("/auth/login", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function register(data: RegisterRequest): Promise<TokenPair> {
  return apiFetch("/auth/register", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export function refreshTokens(refreshToken: string): Promise<TokenPair> {
  return apiFetch("/auth/refresh", {
    method: "POST",
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
}

export function getMe(): Promise<User> {
  return apiFetch("/auth/me");
}

export function updateMe(data: { company_name?: string; home_country?: string }): Promise<User> {
  return apiFetch("/auth/me", {
    method: "PATCH",
    body: JSON.stringify(data),
  });
}
