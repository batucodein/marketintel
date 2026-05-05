"use client";

import { use, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  getCampaign,
  listCampaignContacts,
  approveCampaignContact,
  approveAllCampaignContacts,
  launchCampaign,
  pauseCampaign,
  resumeCampaign,
  stopCampaign,
} from "@/lib/api/outreach";
import {
  ArrowLeft,
  Loader2,
  Play,
  Pause,
  CheckCircle2,
  StopCircle,
} from "lucide-react";

type PageProps = { params: Promise<{ campaignId: string }> };

const STATUS_LABEL: Record<string, string> = {
  pending: "Drafting…",
  drafted: "Awaiting approval",
  approved: "Scheduled",
  sent: "Sent",
  replied: "Replied",
  cold: "Cold",
  skipped: "Skipped",
  failed: "Failed",
};

export default function CampaignDetailPage({ params }: PageProps) {
  const { campaignId } = use(params);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: summary, mutate: refreshSummary } = useSWR(
    `/outreach/campaigns/${campaignId}`,
    () => getCampaign(campaignId),
    { refreshInterval: 5000 },
  );
  const { data: contactsData, mutate: refreshContacts } = useSWR(
    `/outreach/campaigns/${campaignId}/contacts`,
    () => listCampaignContacts(campaignId),
    { refreshInterval: 5000 },
  );

  if (!summary) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading campaign…
      </div>
    );
  }

  async function action(name: string, fn: () => Promise<unknown>) {
    setBusy(name);
    setError(null);
    try {
      await fn();
      await Promise.all([refreshSummary(), refreshContacts()]);
    } catch (e) {
      setError(e instanceof Error ? e.message : `${name} failed`);
    } finally {
      setBusy(null);
    }
  }

  const rows = contactsData?.contacts ?? [];

  const total = summary.total_count;
  const draftedReady = summary.drafted_count;
  const approved = summary.approved_count;
  const sent = summary.sent_count;
  const draftingPct = total > 0 ? Math.round(((total - summary.pending_count) / total) * 100) : 0;

  const isDraft = summary.status === "draft" || summary.status === "ready";
  const isActive = summary.status === "active";
  const isPaused = summary.status === "paused";

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Link href="/outreach/campaigns">
            <Button variant="ghost" size="icon">
              <ArrowLeft className="h-4 w-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold">{summary.name}</h1>
            <p className="text-sm text-muted-foreground">
              {summary.goal || "No goal"} · pace {summary.send_pace_per_day}/day
            </p>
          </div>
        </div>
        <Badge variant={isActive ? "default" : "outline"}>{summary.status}</Badge>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="grid grid-cols-2 md:grid-cols-5 gap-2 text-sm">
        <Stat label="Total" value={total} />
        <Stat label="Drafting" value={summary.pending_count} hint={`${draftingPct}% complete`} />
        <Stat label="Awaiting approval" value={draftedReady} />
        <Stat label="Approved" value={approved} />
        <Stat label="Sent" value={sent} />
      </div>

      <div className="flex flex-wrap gap-2">
        {isDraft && draftedReady > 0 && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => action("approve-all", () => approveAllCampaignContacts(campaignId))}
            disabled={busy !== null}
          >
            <CheckCircle2 className="h-4 w-4 mr-1" /> Approve all drafted
          </Button>
        )}
        {isDraft && approved > 0 && (
          <Button
            size="sm"
            onClick={() => action("launch", () => launchCampaign(campaignId))}
            disabled={busy !== null}
          >
            <Play className="h-4 w-4 mr-1" /> Launch
          </Button>
        )}
        {isActive && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => action("pause", () => pauseCampaign(campaignId))}
            disabled={busy !== null}
          >
            <Pause className="h-4 w-4 mr-1" /> Pause
          </Button>
        )}
        {isPaused && (
          <Button
            size="sm"
            onClick={() => action("resume", () => resumeCampaign(campaignId))}
            disabled={busy !== null}
          >
            <Play className="h-4 w-4 mr-1" /> Resume
          </Button>
        )}
        {(isActive || isPaused) && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => action("stop", () => stopCampaign(campaignId))}
            disabled={busy !== null}
          >
            <StopCircle className="h-4 w-4 mr-1" /> Stop
          </Button>
        )}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Contacts ({rows.length})</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">No contacts on this campaign yet.</p>
          ) : (
            rows.map((row) => (
              <div
                key={row.contact_id}
                className="border rounded-md p-3 flex flex-col gap-1"
              >
                <div className="flex items-start gap-3">
                  <div className="flex-1 min-w-0">
                    <div className="font-medium truncate">{row.ContactName}</div>
                    <div className="text-xs text-muted-foreground truncate">
                      {row.BusinessName} · {row.ContactEmail}
                    </div>
                  </div>
                  <Badge variant="outline" className="text-xs whitespace-nowrap">
                    {STATUS_LABEL[row.status] || row.status}
                  </Badge>
                  {row.status === "drafted" && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        action(`approve-${row.contact_id}`, () =>
                          approveCampaignContact(campaignId, row.contact_id),
                        )
                      }
                      disabled={busy !== null}
                    >
                      Approve
                    </Button>
                  )}
                </div>
                {row.DraftSubject && (
                  <details className="text-xs">
                    <summary className="cursor-pointer text-muted-foreground">
                      {row.DraftSubject}
                    </summary>
                    <pre className="whitespace-pre-wrap mt-1 text-foreground">
                      {row.DraftBodyText}
                    </pre>
                  </details>
                )}
                {row.skip_reason && (
                  <p className="text-xs text-red-600">Skipped: {row.skip_reason}</p>
                )}
                {row.scheduled_send_at && row.status === "approved" && (
                  <p className="text-xs text-muted-foreground">
                    Sends at {new Date(row.scheduled_send_at).toLocaleString()}
                  </p>
                )}
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Stat({
  label,
  value,
  hint,
}: {
  label: string;
  value: number;
  hint?: string;
}) {
  return (
    <Card>
      <CardContent className="py-3">
        <div className="text-2xl font-semibold">{value}</div>
        <div className="text-xs text-muted-foreground">{label}</div>
        {hint && <div className="text-[10px] text-muted-foreground">{hint}</div>}
      </CardContent>
    </Card>
  );
}
