export type SourceStatus = "available" | "partial" | "unavailable" | "not_configured";

export interface SourceReport {
  source: string;
  status: SourceStatus;
  message?: string;
  items?: number;
}

export interface DataQuality {
  sources: SourceReport[];
  overall_completeness: number;
  missing_data_note?: string;
}
