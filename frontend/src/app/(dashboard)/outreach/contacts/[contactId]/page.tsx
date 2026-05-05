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
  listTasks,
  createTask,
  updateTask,
  deleteTask,
} from "@/lib/api/outreach";
import type { Contact, Note, Task } from "@/lib/types/outreach";
import { ArrowLeft, Loader2, Plus, Trash2, CheckCircle2 } from "lucide-react";

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
  const { data: tasksData, mutate: refetchTasks } = useSWR(
    `/outreach/tasks?contact=${contactId}`,
    () => listTasks({ contact_id: contactId }),
  );

  const [newNote, setNewNote] = useState("");
  const [newTask, setNewTask] = useState("");
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

  async function addTask() {
    if (!newTask.trim()) return;
    setBusy(true);
    try {
      await createTask({ title: newTask.trim(), contact_id: contactId });
      setNewTask("");
      refetchTasks();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Add task failed");
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

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Tasks</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            <div className="flex gap-2">
              <input
                value={newTask}
                onChange={(e) => setNewTask(e.target.value)}
                placeholder="Quick task — Enter to add"
                onKeyDown={(e) => e.key === "Enter" && addTask()}
                className="flex-1 rounded-md border border-input bg-background px-3 h-9 text-sm"
              />
              <Button size="sm" onClick={addTask} disabled={busy}>
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            {(tasksData?.tasks ?? []).length === 0 ? (
              <p className="text-sm text-muted-foreground">No tasks yet.</p>
            ) : (
              (tasksData?.tasks ?? []).map((t: Task) => (
                <div key={t.id} className="flex items-center gap-2 px-1 group">
                  <button
                    onClick={async () => {
                      await updateTask(t.id, {
                        title: t.title,
                        body: t.body,
                        due_at: t.due_at,
                        completed_at: t.completed_at ? null : new Date().toISOString(),
                      });
                      refetchTasks();
                    }}
                    title="Toggle complete"
                  >
                    <CheckCircle2
                      className={`h-4 w-4 ${t.completed_at ? "text-green-600" : "text-muted-foreground"}`}
                    />
                  </button>
                  <span
                    className={`text-sm flex-1 ${t.completed_at ? "line-through text-muted-foreground" : ""}`}
                  >
                    {t.title}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="opacity-0 group-hover:opacity-100 h-6 w-6"
                    onClick={async () => {
                      await deleteTask(t.id);
                      refetchTasks();
                    }}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              ))
            )}
          </CardContent>
        </Card>

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
