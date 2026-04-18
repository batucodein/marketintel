"use client";

import { createContext, useContext, useEffect, useState, useCallback } from "react";
import type { User } from "../types/auth";
import { setAccessToken } from "../api/client";
import * as authApi from "../api/auth";

interface AuthContextValue {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, companyName?: string, homeCountry?: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const fetchUser = useCallback(async () => {
    try {
      const me = await authApi.getMe();
      setUser(me);
    } catch {
      setAccessToken(null);
      localStorage.removeItem("refresh_token");
      setUser(null);
    }
  }, []);

  // Try to restore session on mount
  useEffect(() => {
    const refreshToken = localStorage.getItem("refresh_token");
    if (!refreshToken) {
      setIsLoading(false);
      return;
    }

    authApi
      .refreshTokens(refreshToken)
      .then((tokens) => {
        setAccessToken(tokens.access_token);
        localStorage.setItem("refresh_token", tokens.refresh_token);
        return fetchUser();
      })
      .catch(() => {
        localStorage.removeItem("refresh_token");
        setAccessToken(null);
      })
      .finally(() => setIsLoading(false));
  }, [fetchUser]);

  const login = useCallback(async (email: string, password: string) => {
    const tokens = await authApi.login({ email, password });
    setAccessToken(tokens.access_token);
    localStorage.setItem("refresh_token", tokens.refresh_token);
    const me = await authApi.getMe();
    setUser(me);
  }, []);

  const register = useCallback(
    async (email: string, password: string, companyName?: string, homeCountry?: string) => {
      const tokens = await authApi.register({
        email,
        password,
        company_name: companyName,
        home_country: homeCountry,
      });
      setAccessToken(tokens.access_token);
      localStorage.setItem("refresh_token", tokens.refresh_token);
      const me = await authApi.getMe();
      setUser(me);
    },
    [],
  );

  const logout = useCallback(() => {
    setAccessToken(null);
    localStorage.removeItem("refresh_token");
    setUser(null);
  }, []);

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated: !!user,
        isLoading,
        login,
        register,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
