"use client";

import { useMemo, useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Loader2, AlertCircle, CheckCircle2 } from "lucide-react";
import type { ColumnMappingPreview, CanonicalField } from "@/lib/types/discover";

export interface Props {
  preview: ColumnMappingPreview;
  onConfirm: (
    mapping: Record<string, string>,
    overrides: Record<string, string>,
  ) => void;
  onCancel: () => void;
  submitting: boolean;
}

// Sentinel string used in the dropdown for "leave unmapped". Not a valid
// canonical key (canonical keys are snake_case identifiers), so the
// mapping object never carries this value as a real mapping.
const UNMAPPED = "__unmapped__";

export function ColumnMapping({ preview, onConfirm, onCancel, submitting }: Props) {
  // Local mapping state: header → canonical_key (or UNMAPPED). Seeded
  // from the AI's suggestions so the user sees the AI's best guess.
  const [mapping, setMapping] = useState<Record<string, string>>(() => {
    const seed: Record<string, string> = {};
    for (const h of preview.sample.headers) {
      seed[h] = preview.mapping[h] ?? UNMAPPED;
    }
    return seed;
  });

  // Track which headers the user actually changed away from the AI suggestion.
  const overrides = useMemo(() => {
    const out: Record<string, string> = {};
    for (const h of preview.sample.headers) {
      const current = mapping[h];
      const aiSuggested = preview.mapping[h] ?? UNMAPPED;
      if (current !== aiSuggested) {
        out[h] = current === UNMAPPED ? "" : current;
      }
    }
    return out;
  }, [mapping, preview]);

  // Server-truth missing required after applying current local mapping.
  const missingRequired = useMemo(() => {
    const mappedKeys = new Set<string>();
    for (const v of Object.values(mapping)) {
      if (v && v !== UNMAPPED) mappedKeys.add(v);
    }
    return preview.canonical
      .filter((f) => f.required && !mappedKeys.has(f.key))
      .map((f) => f.key);
  }, [mapping, preview]);

  function setHeader(header: string, canonical: string) {
    setMapping({ ...mapping, [header]: canonical });
  }

  function handleConfirm() {
    const final: Record<string, string> = {};
    for (const [h, c] of Object.entries(mapping)) {
      if (c && c !== UNMAPPED) final[h] = c;
    }
    onConfirm(final, overrides);
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-xl font-bold">Confirm column mapping</h2>
        <p className="text-sm text-muted-foreground">
          We detected {preview.sample.headers.length} columns. Review what each one maps to —
          override anything that looks wrong.
        </p>
      </div>

      {missingRequired.length > 0 && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            <strong>Required fields not yet mapped:</strong>{" "}
            {missingRequired.map((k) => (
              <Badge key={k} variant="outline" className="mr-1">{k}</Badge>
            ))}
            <span className="text-xs ml-1">— pick a column for each before importing.</span>
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardContent className="p-0 overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-muted/40">
              <tr>
                <th className="text-left p-3 font-medium">Detected header</th>
                <th className="text-left p-3 font-medium">Sample value</th>
                <th className="text-left p-3 font-medium">Maps to</th>
                <th className="text-left p-3 font-medium">AI confidence</th>
              </tr>
            </thead>
            <tbody>
              {preview.sample.headers.map((h, i) => {
                const sampleValue = preview.sample.samples[0]?.[i] ?? "";
                const conf = preview.confidence[h];
                return (
                  <tr key={h} className="border-t">
                    <td className="p-3 font-mono text-xs">{h}</td>
                    <td className="p-3 text-xs text-muted-foreground truncate max-w-[200px]">
                      {sampleValue || <span className="italic">(empty)</span>}
                    </td>
                    <td className="p-3">
                      <CanonicalSelect
                        value={mapping[h] ?? UNMAPPED}
                        canonical={preview.canonical}
                        onChange={(v) => setHeader(h, v)}
                      />
                    </td>
                    <td className="p-3 text-xs">
                      {conf !== undefined ? (
                        <ConfidencePill value={conf} />
                      ) : (
                        <span className="text-muted-foreground italic">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </CardContent>
      </Card>

      <div className="flex justify-between items-center gap-3">
        <p className="text-xs text-muted-foreground">
          Overall data completeness:{" "}
          <strong>{Math.round(preview.data_completeness * 100)}%</strong>
        </p>
        <div className="flex gap-2">
          <Button variant="outline" onClick={onCancel} disabled={submitting}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={submitting || missingRequired.length > 0}
          >
            {submitting ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" /> Importing…
              </>
            ) : (
              <>
                <CheckCircle2 className="h-4 w-4 mr-2" /> Confirm & import
              </>
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}

function CanonicalSelect({
  value,
  canonical,
  onChange,
}: {
  value: string;
  canonical: CanonicalField[];
  onChange: (v: string) => void;
}) {
  // Group fields visually by their `group` attribute.
  const grouped = useMemo(() => {
    const groups = new Map<string, CanonicalField[]>();
    for (const f of canonical) {
      const arr = groups.get(f.group) ?? [];
      arr.push(f);
      groups.set(f.group, arr);
    }
    return Array.from(groups.entries());
  }, [canonical]);

  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full h-8 rounded-md border border-input bg-background px-2 text-xs"
    >
      <option value={UNMAPPED}>(don&apos;t import this column)</option>
      {grouped.map(([group, fields]) => (
        <optgroup key={group} label={group}>
          {fields.map((f) => (
            <option key={f.key} value={f.key}>
              {f.label}
              {f.required ? " *" : ""}
            </option>
          ))}
        </optgroup>
      ))}
    </select>
  );
}

function ConfidencePill({ value }: { value: number }) {
  const pct = Math.round(value * 100);
  let color = "bg-gray-100 text-gray-700";
  if (value >= 0.85) color = "bg-green-100 text-green-700";
  else if (value >= 0.6) color = "bg-amber-100 text-amber-700";
  else color = "bg-red-100 text-red-700";
  return (
    <span className={`inline-block px-2 py-0.5 rounded text-[11px] font-medium ${color}`}>
      {pct}%
    </span>
  );
}
