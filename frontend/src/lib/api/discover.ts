import { apiFetch } from "./client";
import type { Search, UploadResult } from "../types/discover";

export function uploadExcel(file: File): Promise<UploadResult> {
  const form = new FormData();
  form.append("file", file);
  return apiFetch("/discover/", {
    method: "POST",
    body: form,
  });
}

export function getSearch(searchId: string): Promise<Search> {
  return apiFetch(`/discover/${searchId}`);
}

export function getSearchMarkets(searchId: string): Promise<{
  search_id: string;
  status: string;
  source_file_name: string;
  uploaded_at: string;
  market_ids: string[];
  warnings?: string[];
}> {
  return apiFetch(`/discover/${searchId}/markets`);
}
