"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { GroupList } from "@/components/outreach/group-list";
import { CreateGroupDialog } from "@/components/outreach/create-group-dialog";
import { Plus } from "lucide-react";

// Persistent two-pane shell: the group rail on the left survives navigation
// between a group and its conversations (App Router layouts don't remount).
export default function GroupsLayout({ children }: { children: React.ReactNode }) {
  const [createOpen, setCreateOpen] = useState(false);
  return (
    <div className="grid grid-cols-1 md:grid-cols-[260px_1fr] gap-4">
      <aside className="md:border-r md:pr-3 space-y-2">
        <Button size="sm" className="w-full" onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4 mr-1" /> New group
        </Button>
        <CreateGroupDialog open={createOpen} onOpenChange={setCreateOpen} />
        <GroupList />
      </aside>
      <main className="min-w-0">{children}</main>
    </div>
  );
}
