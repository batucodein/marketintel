import { apiFetch } from "./client";
import type {
  Search,
  UploadResult,
  ColumnMappingPreview,
  CanonicalField,
} from "../types/discover";

export function uploadExcel(file: File): Promise<UploadResult> {
  const form = new FormData();
  form.append("file", file);
  return apiFetch("/discover/", {
    method: "POST",
    body: form,
  });
}

// previewUpload reads the file's headers + first 5 rows, asks the AI to
// suggest a canonical mapping, and returns the preview without persisting
// anything. Caller keeps the File in memory for the subsequent
// importWithMapping call.
export function previewUpload(file: File): Promise<ColumnMappingPreview> {
  const form = new FormData();
  form.append("file", file);
  return apiFetch("/discover/preview", {
    method: "POST",
    body: form,
  });
}

// importWithMapping kicks off the actual ingestion using the user-confirmed
// {header → canonical_key} mapping. Server creates the search row, persists
// the mapping, and runs the enrichment pipeline.
export function importWithMapping(
  file: File,
  mapping: Record<string, string>,
  aiConfidence: Record<string, number>,
  userOverrides: Record<string, string>,
): Promise<UploadResult> {
  const form = new FormData();
  form.append("file", file);
  form.append("mapping", JSON.stringify(mapping));
  form.append("ai_confidence", JSON.stringify(aiConfidence));
  form.append("user_overrides", JSON.stringify(userOverrides));
  return apiFetch("/discover/import", {
    method: "POST",
    body: form,
  });
}

export function getCanonicalFields(): Promise<{ fields: CanonicalField[] }> {
  return apiFetch("/discover/canonical");
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
