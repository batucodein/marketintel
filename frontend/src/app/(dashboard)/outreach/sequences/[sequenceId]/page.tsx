"use client";

import { use, useEffect, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { SequenceStepEditor } from "@/components/outreach/sequence-step-editor";
import {
  getSequence,
  updateSequence,
  replaceSequenceSteps,
  deleteSequence,
} from "@/lib/api/outreach";
import type { SequenceStep } from "@/lib/types/outreach";
import { ArrowLeft, Loader2, Save, Trash2 } from "lucide-react";

type PageProps = { params: Promise<{ sequenceId: string }> };

export default function SequenceDetailPage({ params }: PageProps) {
  const { sequenceId } = use(params);
  const { data, mutate } = useSWR(`/outreach/sequences/${sequenceId}`, () => getSequence(sequenceId));

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [steps, setSteps] = useState<SequenceStep[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  useEffect(() => {
    if (data) {
      setName(data.name);
      setDescription(data.description);
      setSteps(data.steps ?? []);
    }
  }, [data]);

  if (!data) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    );
  }

  async function handleSave() {
    setBusy(true);
    setError(null);
    setMsg(null);
    try {
      await updateSequence(sequenceId, { name, description });
      await replaceSequenceSteps(sequenceId, steps);
      mutate();
      setMsg("Saved");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete() {
    if (!confirm("Delete this sequence? Will fail if active runs exist.")) return;
    setBusy(true);
    setError(null);
    try {
      await deleteSequence(sequenceId);
      window.location.href = "/outreach/sequences";
    } catch (e) {
      setError(e instanceof Error ? e.message : "Delete failed");
      setBusy(false);
    }
  }

  return (
    <div className="max-w-3xl space-y-4">
      <div className="flex items-center gap-2">
        <Link href="/outreach/sequences">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div className="flex-1 min-w-0">
          <h1 className="text-2xl font-bold truncate">{data.name}</h1>
          <p className="text-sm text-muted-foreground">
            {data.active_runs_count > 0
              ? `${data.active_runs_count} active run${data.active_runs_count === 1 ? "" : "s"}`
              : "No active runs"}
          </p>
        </div>
        <Button variant="ghost" size="icon" onClick={handleDelete} disabled={busy}>
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>

      {msg && <Alert><AlertDescription>{msg}</AlertDescription></Alert>}
      {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Basics</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-1">
            <Label>Name</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label>Description</Label>
            <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Steps</CardTitle>
        </CardHeader>
        <CardContent>
          <SequenceStepEditor steps={steps} onChange={setSteps} />
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button onClick={handleSave} disabled={busy}>
          {busy ? (
            <>
              <Loader2 className="h-4 w-4 animate-spin mr-2" /> Saving…
            </>
          ) : (
            <>
              <Save className="h-4 w-4 mr-2" /> Save
            </>
          )}
        </Button>
      </div>
    </div>
  );
}
