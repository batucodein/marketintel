"use client";

import { useState, useRef } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { previewUpload, importWithMapping } from "@/lib/api/discover";
import { ColumnMapping } from "@/components/discover/column-mapping";
import type { ColumnMappingPreview } from "@/lib/types/discover";
import { FileSpreadsheet, Loader2, Upload, X } from "lucide-react";

// Discover flow:
//   pick → previewing (AI mapping in flight) → mapping (user confirms) → importing → redirect
type Step = "pick" | "previewing" | "mapping" | "importing";

export default function DiscoverPage() {
  const router = useRouter();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [step, setStep] = useState<Step>("pick");
  const [error, setError] = useState("");
  const [preview, setPreview] = useState<ColumnMappingPreview | null>(null);

  function handleFileSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const selected = e.target.files?.[0];
    if (!selected) return;
    if (!/\.(xlsx|xls|csv)$/i.test(selected.name)) {
      setError("Only .xlsx, .xls, or .csv files are supported");
      return;
    }
    setFile(selected);
    setError("");
  }

  function handleDrop(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault();
    const dropped = e.dataTransfer.files[0];
    if (!dropped) return;
    if (!/\.(xlsx|xls|csv)$/i.test(dropped.name)) {
      setError("Only .xlsx, .xls, or .csv files are supported");
      return;
    }
    setFile(dropped);
    setError("");
  }

  async function handleAnalyze() {
    if (!file) return;
    setError("");
    setStep("previewing");
    try {
      const p = await previewUpload(file);
      setPreview(p);
      setStep("mapping");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Preview failed");
      setStep("pick");
    }
  }

  async function handleConfirmMapping(
    mapping: Record<string, string>,
    overrides: Record<string, string>,
  ) {
    if (!file || !preview) return;
    setError("");
    setStep("importing");
    try {
      const result = await importWithMapping(file, mapping, preview.confidence, overrides);
      router.push(`/discover/${result.search_id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Import failed");
      setStep("mapping");
    }
  }

  function reset() {
    setFile(null);
    setPreview(null);
    setError("");
    setStep("pick");
  }

  // --- Render ----------------------------------------------------------

  if ((step === "mapping" || step === "importing") && preview) {
    return (
      <div className="max-w-5xl mx-auto py-6">
        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <ColumnMapping
          preview={preview}
          onConfirm={handleConfirmMapping}
          onCancel={reset}
          submitting={step === "importing"}
        />
      </div>
    );
  }

  if (step === "previewing") {
    return (
      <div className="flex flex-col items-center justify-center min-h-[60vh]">
        <Loader2 className="h-8 w-8 animate-spin text-blue-700 mb-4" />
        <p className="text-sm font-medium">Analyzing your file…</p>
        <p className="text-xs text-muted-foreground mt-1">
          Reading headers + asking AI to map them to MarketIntel fields.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center min-h-[60vh]">
      <div className="text-center mb-8">
        <h1 className="text-3xl font-bold text-blue-800 mb-2">Upload Shipment Data</h1>
        <p className="text-muted-foreground">
          Drop any Excel — we&apos;ll map your columns to our fields and call out missing data.
        </p>
      </div>

      <Card className="w-full max-w-lg">
        <CardContent className="pt-6">
          <div className="space-y-4">
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            {!file ? (
              <div
                onDrop={handleDrop}
                onDragOver={(e) => e.preventDefault()}
                onClick={() => fileInputRef.current?.click()}
                className="border-2 border-dashed rounded-lg p-10 text-center cursor-pointer hover:bg-muted/50 transition"
              >
                <Upload className="h-8 w-8 mx-auto mb-3 text-muted-foreground" />
                <p className="text-sm font-medium">Drop your Excel here, or click to browse</p>
                <p className="text-xs text-muted-foreground mt-1">.xlsx, .xls, .csv</p>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".xlsx,.xls,.csv"
                  className="hidden"
                  onChange={handleFileSelect}
                />
              </div>
            ) : (
              <div className="flex items-center gap-3 bg-green-50 border border-green-200 rounded-lg p-4">
                <FileSpreadsheet className="h-6 w-6 text-green-700" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium truncate">{file.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {(file.size / 1024).toFixed(1)} KB
                  </p>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={reset}
                  disabled={(step as Step) === "previewing"}
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            )}

            <Button
              onClick={handleAnalyze}
              disabled={!file || (step as Step) === "previewing"}
              className="w-full"
            >
              {(step as Step) === "previewing" ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin mr-2" />
                  Analyzing…
                </>
              ) : (
                "Analyze Shipments"
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
