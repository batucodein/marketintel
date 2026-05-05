"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { SequenceStepEditor } from "@/components/outreach/sequence-step-editor";
import type { SequenceStep } from "@/lib/types/outreach";
import { createSequence } from "@/lib/api/outreach";
import { ArrowLeft, Loader2, Workflow } from "lucide-react";

export default function NewSequencePage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [steps, setSteps] = useState<SequenceStep[]>([
    { step_number: 1, wait_days: 3, trigger: "no_reply", action: "send_message", prompt_override: null, auto_send: false },
    { step_number: 2, wait_days: 7, trigger: "no_reply", action: "send_message", prompt_override: null, auto_send: false },
    { step_number: 3, wait_days: 14, trigger: "no_reply", action: "mark_cold", prompt_override: null, auto_send: false },
  ]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit() {
    if (!name) {
      setError("Name is required.");
      return;
    }
    if (steps.length === 0) {
      setError("Add at least one step.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const created = await createSequence({ name: name.trim(), description, steps });
      router.push(`/outreach/sequences/${created.id}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Create failed");
      setSubmitting(false);
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
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Workflow className="h-5 w-5" /> New sequence
          </h1>
          <p className="text-sm text-muted-foreground">
            Build a follow-up playbook. Attach it to a campaign or pin it as the default for a contact.
          </p>
        </div>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Basics</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="space-y-1">
            <Label>Name *</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="3-touch follow-up" />
          </div>
          <div className="space-y-1">
            <Label>Description</Label>
            <Textarea
              rows={2}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="3 follow-ups over two weeks, stops on reply"
            />
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

      <div className="flex justify-end gap-2">
        <Link href="/outreach/sequences">
          <Button variant="outline">Cancel</Button>
        </Link>
        <Button onClick={handleSubmit} disabled={submitting || !name}>
          {submitting ? (
            <>
              <Loader2 className="h-4 w-4 animate-spin mr-2" /> Creating…
            </>
          ) : (
            "Create sequence"
          )}
        </Button>
      </div>
    </div>
  );
}
