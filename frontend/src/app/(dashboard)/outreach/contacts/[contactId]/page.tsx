"use client";

import { use, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  getContact,
  updateContact,
  listContactNotes,
  createContactNote,
  deleteContactNote,
} from "@/lib/api/outreach";
import type { Contact, Note } from "@/lib/types/outreach";
import { ArrowLeft, Loader2, Plus, Trash2 } from "lucide-react";

const STAGES = ["lead", "contacted", "replied", "qualified", "won", "lost"] as const;
type Stage = (typeof STAGES)[number];

type PageProps = { params: Promise<{ contactId: string }> };

export default function ContactDetailPage({ params }: PageProps) {
  const { contactId } = use(params);

  const { data: contact, mutate: refetchContact } = useSWR<Contact>(
    `/outreach/contacts/${contactId}`,
    () => getContact(contactId),
  );
  const { data: notesData, mutate: refetchNotes } = useSWR(
    `/outreach/contacts/${contactId}/notes`,
    () => listContactNotes(contactId),
  );

  const [newNote, setNewNote] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (!contact) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading contact…
      </div>
    );
  }

  async function changeStage(stage: Stage) {
    if (!contact) return;
    try {
      await updateContact(contactId, { pipeline_stage: stage });
      refetchContact();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Update failed");
    }
  }

  async function addNote() {
    if (!newNote.trim()) return;
    setBusy(true);
    try {
      await createContactNote(contactId, newNote.trim());
      setNewNote("");
      refetchNotes();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Add note failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="max-w-4xl space-y-4">
      <div className="flex items-center gap-2">
        <Link href="/outreach/contacts">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <div className="flex-1 min-w-0">
          <h1 className="text-2xl font-bold truncate">{contact.display_name}</h1>
          <p className="text-sm text-muted-foreground truncate">
            {contact.primary_email || "no email"} ·{" "}
            <select
              value={contact.pipeline_stage}
              onChange={(e) => changeStage(e.target.value as Stage)}
              className="bg-transparent border rounded px-2 py-0.5 text-sm"
            >
              {STAGES.map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </p>
        </div>
        {contact.unsubscribed_at && (
          <Badge variant="outline" className="text-red-600 border-red-300">
            unsubscribed
          </Badge>
        )}
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="max-w-2xl">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Notes</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="space-y-1">
              <Textarea
                rows={2}
                value={newNote}
                onChange={(e) => setNewNote(e.target.value)}
                placeholder="Quick note — visible only to you"
              />
              <div className="flex justify-end">
                <Button size="sm" onClick={addNote} disabled={busy || !newNote.trim()}>
                  <Plus className="h-4 w-4 mr-1" /> Add note
                </Button>
              </div>
            </div>
            {(notesData?.notes ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">No notes yet.</p>
            ) : (
              (notesData?.notes ?? []).map((n: Note) => (
                <div key={n.id} className="border-t pt-2 group">
                  <div className="flex items-start gap-2">
                    <p className="text-sm flex-1 whitespace-pre-wrap">{n.body}</p>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="opacity-0 group-hover:opacity-100 h-6 w-6"
                      onClick={async () => {
                        await deleteContactNote(contactId, n.id);
                        refetchNotes();
                      }}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                  <p className="text-[10px] text-muted-foreground">
                    {new Date(n.created_at).toLocaleString()}
                  </p>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
