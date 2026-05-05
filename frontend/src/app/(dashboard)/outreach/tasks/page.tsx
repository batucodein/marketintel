"use client";

import { useState } from "react";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  listTasks,
  createTask,
  updateTask,
  deleteTask,
} from "@/lib/api/outreach";
import type { Task } from "@/lib/types/outreach";
import { Loader2, Plus, Trash2, CheckCircle2 } from "lucide-react";

function bucket(t: Task): "overdue" | "today" | "upcoming" | "no-due" | "done" {
  if (t.completed_at) return "done";
  if (!t.due_at) return "no-due";
  const due = new Date(t.due_at).getTime();
  const now = Date.now();
  const startOfTomorrow = new Date();
  startOfTomorrow.setHours(24, 0, 0, 0);
  if (due < now) return "overdue";
  if (due < startOfTomorrow.getTime()) return "today";
  return "upcoming";
}

export default function TasksPage() {
  const { data, mutate } = useSWR("/outreach/tasks", () => listTasks());
  const [title, setTitle] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (!data) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground p-4">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    );
  }

  const tasks = data.tasks ?? [];
  const groups: Record<string, Task[]> = {
    overdue: [],
    today: [],
    upcoming: [],
    "no-due": [],
    done: [],
  };
  for (const t of tasks) groups[bucket(t)].push(t);

  async function handleAdd() {
    if (!title.trim()) {
      setError("Title required");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await createTask({
        title: title.trim(),
        due_at: dueDate ? new Date(dueDate).toISOString() : undefined,
      });
      setTitle("");
      setDueDate("");
      mutate();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Add failed");
    } finally {
      setBusy(false);
    }
  }

  async function toggle(t: Task) {
    await updateTask(t.id, {
      title: t.title,
      body: t.body,
      due_at: t.due_at,
      completed_at: t.completed_at ? null : new Date().toISOString(),
    });
    mutate();
  }

  async function remove(t: Task) {
    if (!confirm("Delete this task?")) return;
    await deleteTask(t.id);
    mutate();
  }

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-bold">Tasks</h1>
        <p className="text-sm text-muted-foreground">
          To-dos tied to your CRM. Group by due date — overdue at the top.
        </p>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">New task</CardTitle>
        </CardHeader>
        <CardContent className="grid grid-cols-1 md:grid-cols-3 gap-2">
          <div className="md:col-span-2 space-y-1">
            <Label className="text-xs">Title</Label>
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Follow up with Acme on quote"
            />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Due date</Label>
            <Input
              type="datetime-local"
              value={dueDate}
              onChange={(e) => setDueDate(e.target.value)}
            />
          </div>
          <div className="md:col-span-3 flex justify-end">
            <Button size="sm" onClick={handleAdd} disabled={busy}>
              <Plus className="h-4 w-4 mr-1" /> Add
            </Button>
          </div>
        </CardContent>
      </Card>

      <Group label="Overdue" tone="bg-red-50" tasks={groups.overdue} onToggle={toggle} onRemove={remove} />
      <Group label="Today" tone="bg-amber-50" tasks={groups.today} onToggle={toggle} onRemove={remove} />
      <Group label="Upcoming" tasks={groups.upcoming} onToggle={toggle} onRemove={remove} />
      <Group label="No due date" tasks={groups["no-due"]} onToggle={toggle} onRemove={remove} />
      <Group label="Done" tone="opacity-60" tasks={groups.done} onToggle={toggle} onRemove={remove} />
    </div>
  );
}

function Group({
  label,
  tone,
  tasks,
  onToggle,
  onRemove,
}: {
  label: string;
  tone?: string;
  tasks: Task[];
  onToggle: (t: Task) => void;
  onRemove: (t: Task) => void;
}) {
  if (tasks.length === 0) return null;
  return (
    <Card className={tone}>
      <CardHeader>
        <CardTitle className="text-base">
          {label} <span className="text-muted-foreground text-xs">({tasks.length})</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">
        {tasks.map((t) => (
          <div
            key={t.id}
            className="flex items-center gap-3 px-2 py-1.5 hover:bg-accent/30 rounded-md group"
          >
            <button onClick={() => onToggle(t)} className="cursor-pointer" title="Toggle complete">
              <CheckCircle2
                className={`h-5 w-5 ${t.completed_at ? "text-green-600" : "text-muted-foreground"}`}
              />
            </button>
            <div className="flex-1 min-w-0">
              <div className={`text-sm ${t.completed_at ? "line-through text-muted-foreground" : ""}`}>
                {t.title}
              </div>
              {t.due_at && (
                <div className="text-xs text-muted-foreground">
                  Due {new Date(t.due_at).toLocaleString()}
                </div>
              )}
            </div>
            <Button
              variant="ghost"
              size="icon"
              className="opacity-0 group-hover:opacity-100"
              onClick={() => onRemove(t)}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}
