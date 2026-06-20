"use client";

import { useState, useMemo, useEffect } from "react";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { getMarkets } from "@/lib/api/markets";
import { listSenderProfiles } from "@/lib/api/outreach";
import {
  listPersonas,
  listSimulations,
  runSimulation,
  getSimulation,
  deleteSimulation,
  advanceSimulation,
  approveSimLead,
  dismissSimLead,
  editSimDraft,
  refineSimDraft,
  setSimPlaybook,
  getSimAssistant,
  sendSimAssistantMessage,
  confirmSimAssistantMessage,
  dismissSimAssistantMessage,
} from "@/lib/api/simulation";
import { AssistantDock, type AssistantSource } from "@/components/outreach/draft-assistant-dock";
import type { Market } from "@/lib/types/market";
import type { SenderProfile, SequenceStep } from "@/lib/types/outreach";
import type { Simulation, SimulationLead } from "@/lib/types/simulation";
import { FollowupStepEditor } from "@/components/outreach/followup-step-editor";
import { PlaybookEditor } from "@/components/outreach/playbook-editor";
import { SentimentBadge } from "@/components/outreach/sentiment-badge";
import { TagChip } from "@/components/outreach/tag-chip";
import { FacetChip } from "@/components/outreach/facet-filter-bar";
import { SENTIMENT_ORDER, SENTIMENT_META, INTENT_TAGS, tagLabel, levelFromLegacy } from "@/components/outreach/intent-tags";
import { useOutreachEvents } from "@/lib/sse/use-outreach-events";
import type { SentimentLevel, PlaybookEntry } from "@/lib/types/outreach";
import {
  Loader2,
  FlaskConical,
  ChevronDown,
  ChevronRight,
  Trash2,
  Play,
  FastForward,
  Check,
  X,
  Sparkles,
  Clock,
} from "lucide-react";

// Every outcome deriveOutcome (backend simulation/service.go) can produce, with
// light + dark variants (same pattern as SENTIMENT_META in intent-tags.ts).
const OUTCOME_COLORS: Record<string, string> = {
  replied_positive: "border-green-300 text-green-700 bg-green-50 dark:bg-green-500/15 dark:text-green-300",
  replied: "border-blue-300 text-blue-700 bg-blue-50 dark:bg-blue-500/15 dark:text-blue-300",
  replied_negative: "border-amber-300 text-amber-700 bg-amber-50 dark:bg-amber-500/15 dark:text-amber-300",
  no_reply: "border-gray-300 text-gray-600 bg-gray-50 dark:bg-gray-500/15 dark:text-gray-300",
  cold: "border-slate-300 text-slate-600 bg-slate-50 dark:bg-slate-500/15 dark:text-slate-300",
  unsubscribed: "border-red-300 text-red-700 bg-red-50 dark:bg-red-500/15 dark:text-red-300",
  skipped_no_email: "border-gray-300 text-gray-500 bg-gray-50 dark:bg-gray-500/15 dark:text-gray-400",
};

// simAssistantSource binds the shared AssistantDock to a simulation's agent.
function simAssistantSource(sim: Simulation): AssistantSource {
  return {
    id: sim.id,
    swrKey: `/outreach/simulations/${sim.id}/assistant`,
    eventKind: "simulation_progress",
    playbook: Object.entries(sim.playbook ?? {}).map(([tag, instruction]) => ({ tag, instruction })),
    loadHistory: () => getSimAssistant(sim.id),
    send: (t) => sendSimAssistantMessage(sim.id, t),
    confirm: (mid) => confirmSimAssistantMessage(sim.id, mid),
    dismiss: (mid) => dismissSimAssistantMessage(sim.id, mid),
  };
}

export default function SimulationPage() {
  const { data: markets } = useSWR<Market[]>("/markets", () => getMarkets());
  const { data: profiles } = useSWR<SenderProfile[]>("/outreach/sender-profile", () =>
    listSenderProfiles(),
  );
  const { data: personaData } = useSWR("/outreach/simulations/personas", () => listPersonas());
  const { data: historyData, mutate: refreshHistory } = useSWR("/outreach/simulations", () =>
    listSimulations(),
  );

  const [name, setName] = useState("");
  const [marketId, setMarketId] = useState("");
  const [brandId, setBrandId] = useState("");
  const [leadCount, setLeadCount] = useState(6);
  const [personas, setPersonas] = useState<string[]>([]);
  const [steps, setSteps] = useState<SequenceStep[]>([
    { step_number: 1, wait_days: 0, trigger: "no_reply", action: "send_message", prompt_override: null, auto_send: false },
    { step_number: 2, wait_days: 3, trigger: "no_reply", action: "send_message", prompt_override: null, auto_send: false },
    { step_number: 3, wait_days: 5, trigger: "no_reply", action: "mark_cold", prompt_override: null, auto_send: false },
  ]);

  const [playbook, setPlaybook] = useState<PlaybookEntry[]>([]);
  const [includeLessons, setIncludeLessons] = useState(true);
  const [onPos, setOnPos] = useState("auto_draft_reply");
  const [onNeg, setOnNeg] = useState("mark_cold");
  const [quickMode, setQuickMode] = useState(false);
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [result, setResult] = useState<Simulation | null>(null);
  const [error, setError] = useState<string | null>(null);

  useOutreachEvents((e) => {
    if (e.kind === "simulation_progress" && e.data) {
      setProgress({ done: Number(e.data.done), total: Number(e.data.total) });
    }
  });

  const allPersonas = personaData?.personas ?? [];
  const effectivePersonas = personas.length > 0 ? personas : allPersonas.map((p) => p.key);

  function togglePersona(key: string) {
    setPersonas((cur) => (cur.includes(key) ? cur.filter((k) => k !== key) : [...cur, key]));
  }

  async function run() {
    if (!marketId || !brandId) {
      setError("Pick a market and a brand.");
      return;
    }
    // Clamp to the backend's accepted range (service.go Run: 1..25, else it
    // silently resets to 8) and reflect the clamped value in the input.
    const clampedLeads = Math.min(25, Math.max(1, Math.floor(leadCount) || 1));
    if (clampedLeads !== leadCount) setLeadCount(clampedLeads);
    setRunning(true);
    setError(null);
    setResult(null);
    setProgress({ done: 0, total: clampedLeads });
    try {
      const body = {
        name: name.trim() || "Simulation",
        mode: quickMode ? ("quick" as const) : ("interactive" as const),
        market_id: marketId,
        brand_id: brandId,
        lead_count: clampedLeads,
        personas: effectivePersonas,
        steps,
        on_positive_action: onPos,
        on_negative_action: onNeg,
        playbook: Object.fromEntries(
          playbook.filter((p) => p.instruction.trim()).map((p) => [p.tag, p.instruction.trim()]),
        ),
        include_lessons: includeLessons,
      };
      const sim = await runSimulation(body);
      setResult(sim);
      refreshHistory();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Simulation failed");
    } finally {
      setRunning(false);
      setProgress(null);
    }
  }

  async function openSim(id: string) {
    setError(null);
    try {
      setResult(await getSimulation(id));
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load");
    }
  }

  async function removeSim(id: string) {
    if (!confirm("Delete this simulation run?")) return;
    try {
      await deleteSimulation(id);
      if (result?.id === id) setResult(null);
      refreshHistory();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  return (
    <div className="space-y-4 max-w-5xl">
      <div>
        <h1 className="text-2xl font-bold flex items-center gap-2">
          <FlaskConical className="h-6 w-6 text-primary" /> Simulation Lab
        </h1>
        <p className="text-sm text-muted-foreground">
          Test your drafts + follow-ups against AI buyer-personas across every scenario — in
          seconds, with zero real emails sent.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {/* Config */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">New run</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label>Name</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Marble Q3 test" />
            </div>
            <div className="space-y-1">
              <Label>Leads</Label>
              <Input
                type="number"
                min={1}
                max={25}
                value={leadCount}
                onChange={(e) => setLeadCount(Number(e.target.value))}
              />
            </div>
            <div className="space-y-1">
              <Label>Market (lead source)</Label>
              <select
                value={marketId}
                onChange={(e) => setMarketId(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="">— Select a market —</option>
                {(markets ?? []).map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.derived_product_name ?? m.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="space-y-1">
              <Label>Brand (sends as)</Label>
              <select
                value={brandId}
                onChange={(e) => setBrandId(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="">— Select a brand —</option>
                {(profiles ?? []).map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name || p.company_name}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div className="space-y-1">
            <Label>Personas (leave all unchecked = use every persona)</Label>
            <div className="flex flex-wrap gap-2">
              {allPersonas.map((p) => (
                <button
                  key={p.key}
                  onClick={() => togglePersona(p.key)}
                  className={
                    "rounded-full border px-3 py-1 text-xs transition " +
                    (personas.includes(p.key)
                      ? "border-blue-400 bg-blue-50 text-blue-700"
                      : "hover:border-blue-300")
                  }
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-1">
            <Label>If no reply — cadence</Label>
            <FollowupStepEditor steps={steps} onChange={setSteps} />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label>If they reply positively</Label>
              <select
                value={onPos}
                onChange={(e) => setOnPos(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="auto_draft_reply">Auto-draft a reply for approval</option>
                <option value="notify">Notify me to take over</option>
              </select>
            </div>
            <div className="space-y-1">
              <Label>If they reply negatively</Label>
              <select
                value={onNeg}
                onChange={(e) => setOnNeg(e.target.value)}
                className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
              >
                <option value="mark_cold">Stop &amp; mark cold</option>
                <option value="notify">Notify me to review</option>
              </select>
            </div>
          </div>

          <div className="space-y-1">
            <Label>Reply playbook (test how standing instructions shape drafts)</Label>
            <PlaybookEditor entries={playbook} onChange={setPlaybook} />
          </div>

          <label className="flex items-center gap-2 text-xs cursor-pointer">
            <input
              type="checkbox"
              checked={includeLessons}
              onChange={(e) => setIncludeLessons(e.target.checked)}
              className="h-3.5 w-3.5 accent-blue-600"
            />
            Apply saved brand lessons (production will)
          </label>

          <label className="flex items-center gap-2 text-xs cursor-pointer">
            <input
              type="checkbox"
              checked={quickMode}
              onChange={(e) => setQuickMode(e.target.checked)}
              className="h-3.5 w-3.5 accent-blue-600"
            />
            Quick run (no pauses — run the whole cadence at once)
          </label>

          <Button onClick={run} disabled={running}>
            {running ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
                {quickMode ? "Running" : "Drafting"} {progress ? `${progress.done}/${progress.total}` : ""}…
              </>
            ) : (
              <>
                <Play className="h-4 w-4 mr-1" /> {quickMode ? "Run simulation" : "Start stepped run"}
              </>
            )}
          </Button>
        </CardContent>
      </Card>

      {result && <ResultView sim={result} onChange={setResult} />}

      {result && result.mode === "interactive" && (
        <AssistantDock
          source={simAssistantSource(result)}
          onChanged={async () => {
            try {
              setResult(await getSimulation(result.id));
            } catch {
              /* refresh best-effort */
            }
          }}
        />
      )}

      {/* History */}
      {(historyData?.simulations?.length ?? 0) > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Past runs</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1">
            {(historyData?.simulations ?? []).map((s) => (
              <div key={s.id} className="flex items-center gap-2 group">
                <button
                  onClick={() => openSim(s.id)}
                  className="flex-1 text-left rounded px-2 py-1.5 text-sm hover:bg-accent"
                >
                  <span className="font-medium">{s.name || "Simulation"}</span>{" "}
                  <span className="text-xs text-muted-foreground">
                    · {s.status} · {new Date(s.created_at).toLocaleString()}
                  </span>
                </button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-7 w-7 opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-red-600"
                  onClick={() => removeSim(s.id)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

// lastReply returns the sentiment level + tags of a lead's most recent inbound
// reply — the same signals the live inbox filters/badges on. Falls back to the
// legacy 3-value sentiment when no 5-level value is set (same as the badge).
function lastReply(lead: SimulationLead): { level: SentimentLevel | null; tags: string[] } {
  const t = lead.transcript ?? [];
  for (let i = t.length - 1; i >= 0; i--) {
    if (t[i].who === "them")
      return {
        level: t[i].sentiment_level ?? levelFromLegacy(t[i].sentiment),
        tags: t[i].tags ?? [],
      };
  }
  return { level: null, tags: [] };
}

interface SimFacets {
  sentiment: SentimentLevel[];
  outcomes: string[];
  tags: string[];
}

function ResultView({ sim, onChange }: { sim: Simulation; onChange: (s: Simulation) => void }) {
  const s = sim.summary;
  const leads = useMemo(() => sim.leads ?? [], [sim.leads]);
  const [facets, setFacets] = useState<SimFacets>({ sentiment: [], outcomes: [], tags: [] });
  const [advancing, setAdvancing] = useState(false);

  const interactive = sim.mode === "interactive";
  const pending = useMemo(() => leads.filter((l) => l.state === "awaiting_approval"), [leads]);
  const doneCount = leads.filter((l) => l.state === "done").length;
  const snoozed = leads.filter((l) => l.state === "snoozed").length;

  async function advance() {
    setAdvancing(true);
    try {
      onChange(await advanceSimulation(sim.id));
    } catch (e) {
      alert(e instanceof Error ? e.message : "Advance failed");
    } finally {
      setAdvancing(false);
    }
  }

  // Per-lead signals (last reply) + facet counts, computed client-side — this is
  // the same triage view as the real inbox, on simulated data.
  const enriched = useMemo(
    () => leads.map((l) => ({ lead: l, ...lastReply(l) })),
    [leads],
  );
  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const e of enriched) {
      if (e.level) c[e.level] = (c[e.level] ?? 0) + 1;
      c[e.lead.outcome] = (c[e.lead.outcome] ?? 0) + 1;
      for (const t of e.tags) c[t] = (c[t] ?? 0) + 1;
    }
    return c;
  }, [enriched]);

  const outcomeKeys = useMemo(
    () => Array.from(new Set(leads.map((l) => l.outcome))),
    [leads],
  );
  const visibleTags = INTENT_TAGS.filter(
    (t) => (counts[t.value] ?? 0) > 0 || facets.tags.includes(t.value),
  );

  // OR within a facet, AND across facets — identical semantics to the inbox.
  const filtered = enriched.filter((e) => {
    if (facets.sentiment.length && !(e.level && facets.sentiment.includes(e.level))) return false;
    if (facets.outcomes.length && !facets.outcomes.includes(e.lead.outcome)) return false;
    if (facets.tags.length && !e.tags.some((t) => facets.tags.includes(t))) return false;
    return true;
  });

  function toggle<T extends string>(list: T[], v: T): T[] {
    return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
  }
  const anyActive = facets.sentiment.length || facets.outcomes.length || facets.tags.length;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2 flex-wrap">
          <CardTitle className="text-base flex-1">
            {interactive ? "Stepped run" : "Results"} — {sim.name || "Simulation"}
          </CardTitle>
          {interactive && (
            <>
              <Badge variant="outline" className="text-[10px] gap-1">
                <Clock className="h-3 w-3" /> Day {sim.virtual_day}
              </Badge>
              <Badge variant="outline" className="text-[10px]">
                {doneCount}/{leads.length} done
              </Badge>
              {sim.status !== "done" && (
                <Button size="sm" onClick={advance} disabled={advancing} title="Advance the cadence to the next touch">
                  {advancing ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" />
                  ) : (
                    <FastForward className="h-3.5 w-3.5 mr-1" />
                  )}
                  Advance
                </Button>
              )}
            </>
          )}
        </div>
        {interactive && (
          <p className="text-[11px] text-muted-foreground">
            {sim.status === "done"
              ? "Run complete — every lead reached a terminal state."
              : pending.length > 0
                ? `${pending.length} draft${pending.length > 1 ? "s" : ""} awaiting your approval. Approve, edit, or refine them${snoozed ? `, and ${snoozed} lead(s) snoozed` : ""}, then Advance.`
                : `Click Advance to move the cadence forward${snoozed ? ` (${snoozed} snoozed)` : ""}.`}
          </p>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {s && (
          <div className="flex flex-wrap gap-4 text-sm">
            <Stat label="Leads" value={String(s.lead_count)} />
            <Stat label="Avg quality" value={`${s.avg_score}/100`} />
            <Stat label="Human-feel" value={`${s.avg_human_feel}/100`} />
            {s.sentiment_accuracy != null && (
              <Stat label="Sentiment accuracy" value={`${s.sentiment_accuracy}%`} />
            )}
          </div>
        )}

        <SimCadencePanel sim={sim} />

        {/* Approvals — intervene before each touch sends, exactly like the live group. */}
        {interactive && pending.length > 0 && (
          <div className="space-y-2">
            <p className="text-xs font-medium flex items-center gap-1.5">
              <Sparkles className="h-3.5 w-3.5 text-primary" /> Awaiting approval ({pending.length})
            </p>
            {pending.map((l) => (
              <PendingDraftCard key={l.id} sim={sim} lead={l} onChange={onChange} />
            ))}
          </div>
        )}

        {interactive && <SimPlaybookSection sim={sim} onChange={onChange} />}

        {/* Faceted preview — mirrors the live inbox triage UX on simulated data. */}
        <div className="space-y-2 rounded-md border bg-muted/20 p-3">
          <p className="text-[11px] text-muted-foreground">
            Preview triage — filter exactly like the live inbox before you send anything.
          </p>
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[11px] uppercase tracking-wide text-muted-foreground/70 w-16 shrink-0">
              Sentiment
            </span>
            {SENTIMENT_ORDER.map((lvl) => (
              <FacetChip
                key={lvl}
                label={`${SENTIMENT_META[lvl].icon} ${SENTIMENT_META[lvl].label}`}
                count={counts[lvl]}
                active={facets.sentiment.includes(lvl)}
                onClick={() => setFacets({ ...facets, sentiment: toggle(facets.sentiment, lvl) })}
              />
            ))}
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-[11px] uppercase tracking-wide text-muted-foreground/70 w-16 shrink-0">
              Outcome
            </span>
            {outcomeKeys.map((o) => (
              <FacetChip
                key={o}
                label={o.replace(/_/g, " ")}
                count={counts[o]}
                active={facets.outcomes.includes(o)}
                onClick={() => setFacets({ ...facets, outcomes: toggle(facets.outcomes, o) })}
              />
            ))}
          </div>
          {visibleTags.length > 0 && (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[11px] uppercase tracking-wide text-muted-foreground/70 w-16 shrink-0">
                Intent
              </span>
              {visibleTags.map((t) => (
                <FacetChip
                  key={t.value}
                  label={tagLabel(t.value)}
                  count={counts[t.value]}
                  active={facets.tags.includes(t.value)}
                  onClick={() => setFacets({ ...facets, tags: toggle(facets.tags, t.value) })}
                />
              ))}
            </div>
          )}
          {anyActive ? (
            <button
              type="button"
              onClick={() => setFacets({ sentiment: [], outcomes: [], tags: [] })}
              className="text-[11px] text-muted-foreground hover:text-foreground underline underline-offset-2 cursor-pointer"
            >
              Clear filters
            </button>
          ) : null}
        </div>

        <div className="space-y-2">
          {filtered.length === 0 ? (
            <p className="text-sm text-muted-foreground py-6 text-center">No leads in this filter.</p>
          ) : (
            filtered.map((e) => (
              <LeadRow key={e.lead.id} lead={e.lead} level={e.level} tags={e.tags} />
            ))
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// STATE_LABELS gives each live lead state a short, human chip.
const STATE_LABELS: Record<string, { label: string; cls: string }> = {
  awaiting_approval: { label: "awaiting you", cls: "border-primary/40 text-primary bg-primary/10" },
  active: { label: "in cadence", cls: "border-blue-300 text-blue-700 bg-blue-50 dark:bg-blue-500/15 dark:text-blue-300" },
  snoozed: { label: "snoozed", cls: "border-amber-300 text-amber-700 bg-amber-50 dark:bg-amber-500/15 dark:text-amber-300" },
  done: { label: "done", cls: "border-gray-300 text-gray-500 bg-gray-50 dark:bg-gray-500/15 dark:text-gray-300" },
};

// PendingDraftCard lets the user intervene on one lead's pending draft before it
// "sends": edit it, refine it with an instruction (real AI), approve it (which
// advances the lead — the persona may reply), or dismiss it.
function PendingDraftCard({
  sim,
  lead,
  onChange,
}: {
  sim: Simulation;
  lead: SimulationLead;
  onChange: (s: Simulation) => void;
}) {
  const pd = lead.pending_draft;
  const [editing, setEditing] = useState(false);
  const [subject, setSubject] = useState(pd?.subject ?? "");
  const [body, setBody] = useState(pd?.body ?? "");
  const [refine, setRefine] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  if (!pd) return null;

  const kindLabel = pd.kind === "cold" ? "Cold opener" : pd.kind === "reply" ? "Reply" : "Follow-up";

  async function act(fn: () => Promise<Simulation>, tag: string) {
    setBusy(tag);
    try {
      onChange(await fn());
    } catch (e) {
      alert(e instanceof Error ? e.message : "Action failed");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="border rounded-md p-3 space-y-2 bg-background">
      <div className="flex items-center gap-2 text-sm">
        <span className="font-medium truncate">{lead.display_name}</span>
        <span className="text-xs text-muted-foreground">{lead.persona.replace(/_/g, " ")}</span>
        <Badge variant="outline" className="text-[10px] ml-auto">
          {kindLabel}
        </Badge>
      </div>

      {editing ? (
        <div className="space-y-2">
          <Input value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="Subject" className="text-sm" />
          <Textarea value={body} onChange={(e) => setBody(e.target.value)} rows={6} className="text-sm" />
          <div className="flex gap-2">
            <Button
              size="sm"
              disabled={busy !== null}
              onClick={() =>
                act(() => editSimDraft(sim.id, lead.id, { subject, body }).then((s) => (setEditing(false), s)), "save")
              }
            >
              Save
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <div className="rounded bg-muted/40 px-3 py-2 text-sm">
          {pd.subject && <div className="text-[11px] opacity-80 mb-1">Subject: {pd.subject}</div>}
          <div className="whitespace-pre-wrap max-h-48 overflow-y-auto">{pd.body}</div>
        </div>
      )}

      <div className="flex items-center gap-2">
        <Input
          value={refine}
          onChange={(e) => setRefine(e.target.value)}
          placeholder="Refine — e.g. warmer, shorter, mention MOQ"
          className="text-xs h-8"
          onKeyDown={(e) => {
            if (e.key === "Enter" && refine.trim()) {
              act(() => refineSimDraft(sim.id, lead.id, refine.trim()).then((s) => (setRefine(""), s)), "refine");
            }
          }}
        />
        <Button
          size="sm"
          variant="outline"
          disabled={busy !== null || !refine.trim()}
          onClick={() => act(() => refineSimDraft(sim.id, lead.id, refine.trim()).then((s) => (setRefine(""), s)), "refine")}
        >
          {busy === "refine" ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Sparkles className="h-3.5 w-3.5" />}
        </Button>
      </div>

      <div className="flex gap-2">
        <Button size="sm" disabled={busy !== null} onClick={() => act(() => approveSimLead(sim.id, lead.id), "approve")}>
          {busy === "approve" ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : <Check className="h-3.5 w-3.5 mr-1" />}
          Approve
        </Button>
        {!editing && (
          <Button size="sm" variant="ghost" onClick={() => { setSubject(pd.subject); setBody(pd.body); setEditing(true); }}>
            Edit
          </Button>
        )}
        <Button
          size="sm"
          variant="ghost"
          className="text-muted-foreground hover:text-red-600"
          disabled={busy !== null}
          onClick={() => act(() => dismissSimLead(sim.id, lead.id), "dismiss")}
        >
          <X className="h-3.5 w-3.5 mr-1" /> Dismiss
        </Button>
      </div>
    </div>
  );
}

function LeadRow({
  lead,
  level,
  tags = [],
}: {
  lead: SimulationLead;
  level?: SentimentLevel | null;
  tags?: string[];
}) {
  const [open, setOpen] = useState(false);
  const st = STATE_LABELS[lead.state];
  return (
    <div className="border rounded-md">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-accent/50"
      >
        {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        <span className="font-medium truncate">{lead.display_name}</span>
        <span className="text-xs text-muted-foreground">{lead.persona.replace(/_/g, " ")}</span>
        {st && lead.state !== "done" && (
          <Badge variant="outline" className={"text-[10px] " + st.cls}>
            {st.label}
          </Badge>
        )}
        {(lead.state === "active" || lead.state === "snoozed") && (
          <span className="text-[10px] text-muted-foreground whitespace-nowrap">
            step {lead.current_step + 1} · day {lead.state === "snoozed" ? lead.snooze_until_day : lead.next_day}
          </span>
        )}
        {level && <SentimentBadge label={level} />}
        {tags.slice(0, 3).map((t) => (
          <TagChip key={t} tag={t} />
        ))}
        <Badge variant="outline" className={"ml-auto text-[10px] " + (OUTCOME_COLORS[lead.outcome] ?? "")}>
          {lead.outcome.replace(/_/g, " ")}
        </Badge>
        {lead.grade && (
          <Badge variant="outline" className="text-[10px]">
            {lead.grade.score}/100
          </Badge>
        )}
      </button>
      {open && (
        <div className="border-t p-3 space-y-3">
          {lead.grade && (
            <div className="text-xs bg-muted/40 rounded p-2">
              <span className="font-medium">Judge:</span> human-feel {lead.grade.human_feel},
              correctness {lead.grade.correctness}. {lead.grade.notes}
            </div>
          )}
          <div className="space-y-2">
            {(lead.transcript ?? []).map((m, i) =>
              m.who === "event" ? (
                <div key={i} className="text-center text-[11px] text-muted-foreground italic">
                  · {m.body} (day {m.day}) ·
                </div>
              ) : (
                <div key={i} className={"flex " + (m.who === "you" ? "justify-end" : "justify-start")}>
                  <div
                    className={
                      "max-w-[85%] rounded-lg px-3 py-2 text-sm " +
                      (m.who === "you" ? "bg-primary text-primary-foreground" : "bg-muted text-foreground")
                    }
                  >
                    {m.subject && <div className="text-[11px] opacity-80 mb-1">Subject: {m.subject}</div>}
                    <div className="whitespace-pre-wrap">{m.body}</div>
                    {m.who === "them" && (m.sentiment_level || m.sentiment || m.tags?.length) && (
                      <div className="flex flex-wrap items-center gap-1 mt-1.5">
                        <SentimentBadge label={m.sentiment_level} legacy={m.sentiment} />
                        {m.tags?.map((t) => <TagChip key={t} tag={t} />)}
                      </div>
                    )}
                    <div className="text-[10px] opacity-70 mt-1">day {m.day}</div>
                  </div>
                </div>
              ),
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// SimPlaybookSection is the mid-run, manual playbook editor — the same standing
// reply guidance the agent manages by chat, editable directly during a run.
function SimPlaybookSection({ sim, onChange }: { sim: Simulation; onChange: (s: Simulation) => void }) {
  const [open, setOpen] = useState(false);
  const [entries, setEntries] = useState<PlaybookEntry[]>(() =>
    Object.entries(sim.playbook ?? {}).map(([tag, instruction]) => ({ tag, instruction })),
  );
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  // Re-seed from server state (e.g. after the agent edits it) unless the user is
  // mid-edit.
  useEffect(() => {
    if (!dirty) {
      setEntries(Object.entries(sim.playbook ?? {}).map(([tag, instruction]) => ({ tag, instruction })));
    }
  }, [sim.playbook, dirty]);

  const count = entries.filter((e) => e.instruction.trim()).length;

  async function save() {
    setSaving(true);
    try {
      const rec = Object.fromEntries(
        entries.filter((e) => e.instruction.trim()).map((e) => [e.tag, e.instruction.trim()]),
      );
      onChange(await setSimPlaybook(sim.id, rec));
      setDirty(false);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="rounded-md border">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-accent/50"
      >
        {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        <span className="font-medium">Reply playbook</span>
        <span className="text-xs text-muted-foreground">({count})</span>
      </button>
      {open && (
        <div className="border-t p-3 space-y-2">
          <p className="text-[11px] text-muted-foreground">
            Standing instructions the AI follows when drafting replies — edit them mid-run to see the
            effect on the next reply draft.
          </p>
          <PlaybookEditor
            entries={entries}
            onChange={(e) => {
              setEntries(e);
              setDirty(true);
            }}
          />
          <Button size="sm" onClick={save} disabled={saving || !dirty}>
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : null}
            Save playbook
          </Button>
        </div>
      )}
    </div>
  );
}

// SimCadencePanel mirrors the live group's Cadence dashboard on simulated data:
// the per-touch funnel, outcomes, completion %, and what's queued by virtual day.
// Computed entirely from the loaded leads — no backend call.
function SimCadencePanel({ sim }: { sim: Simulation }) {
  const [open, setOpen] = useState(false);
  const leads = sim.leads ?? [];
  const total = leads.length;
  if (total === 0) return null;

  const youCounts = leads.map((l) => (l.transcript ?? []).filter((m) => m.who === "you").length);
  const maxTouch = Math.max(1, 1 + (sim.steps?.length ?? 0));
  const pct = (n: number) => (total ? Math.round((n / total) * 1000) / 10 : 0);

  const funnel: { label: string; sent: number; pct: number }[] = [];
  for (let k = 0; k < maxTouch; k++) {
    const sent = youCounts.filter((c) => c > k).length;
    funnel.push({ label: k === 0 ? "Cold opener" : `Follow-up ${k}`, sent, pct: pct(sent) });
  }
  const maxSent = Math.max(1, ...funnel.map((f) => f.sent));

  const outcomes: Record<string, number> = {};
  for (const l of leads) if (l.outcome) outcomes[l.outcome] = (outcomes[l.outcome] ?? 0) + 1;
  const doneCount = leads.filter((l) => l.state === "done").length;

  const upMap: Record<number, number> = {};
  for (const l of leads) {
    if (l.state === "active" || l.state === "awaiting_approval") upMap[l.next_day] = (upMap[l.next_day] ?? 0) + 1;
    else if (l.state === "snoozed" && l.snooze_until_day != null)
      upMap[l.snooze_until_day] = (upMap[l.snooze_until_day] ?? 0) + 1;
  }
  const upcoming = Object.entries(upMap)
    .map(([d, n]) => ({ day: Number(d), n }))
    .sort((a, b) => a.day - b.day);

  return (
    <div className="rounded-md border">
      <button
        onClick={() => setOpen((o) => !o)}
        className="w-full flex items-center gap-2 px-3 py-2 text-sm hover:bg-accent/50"
      >
        {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        <span className="font-medium">Cadence progress</span>
        <span className="text-xs text-muted-foreground ml-auto">
          {pct(doneCount)}% complete · {total} leads
        </span>
      </button>
      {open && (
        <div className="border-t p-3 space-y-4 text-sm">
          <div className="space-y-1.5">
            <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Touches sent</p>
            {funnel.map((f) => (
              <div key={f.label} className="flex items-center gap-2">
                <span className="w-24 shrink-0 text-xs truncate">{f.label}</span>
                <div className="flex-1 h-4 rounded bg-muted overflow-hidden">
                  <div className="h-full bg-primary/70" style={{ width: `${(f.sent / maxSent) * 100}%` }} />
                </div>
                <span className="w-16 shrink-0 text-right text-xs text-muted-foreground">
                  {f.sent} ({f.pct}%)
                </span>
              </div>
            ))}
          </div>
          <div className="space-y-1">
            <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Outcomes</p>
            <div className="flex flex-wrap gap-2">
              {Object.entries(outcomes).map(([k, n]) => (
                <span key={k} className="rounded-md border bg-muted/30 px-2 py-1 text-xs">
                  {k.replace(/_/g, " ")}: <span className="font-medium">{n}</span>
                </span>
              ))}
            </div>
          </div>
          {upcoming.length > 0 && (
            <div className="space-y-1">
              <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Upcoming (by virtual day)</p>
              <div className="flex flex-wrap gap-2">
                {upcoming.map((u) => (
                  <span key={u.day} className="rounded-md border bg-muted/30 px-2 py-1 text-xs">
                    Day {u.day}: <span className="font-medium">{u.n}</span>
                  </span>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-lg font-semibold">{value}</div>
      <div className="text-xs text-muted-foreground">{label}</div>
    </div>
  );
}
