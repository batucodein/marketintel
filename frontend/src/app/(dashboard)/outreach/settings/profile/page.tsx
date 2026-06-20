"use client";

import { useState } from "react";
import useSWR from "swr";
import { Button } from "@/components/ui/button";
import { listSenderProfiles, deleteSenderProfile } from "@/lib/api/outreach";
import type { SenderProfile } from "@/lib/types/outreach";
import { SenderProfileEditor } from "@/components/outreach/sender-profile-editor";
import { Loader2, Plus, Trash2 } from "lucide-react";

// NEW = sentinel for the "create a brand" selection.
const NEW = "__new__";

export default function SenderProfilesPage() {
  const { data: profiles, mutate, isLoading } = useSWR<SenderProfile[]>(
    "/outreach/sender-profile",
    () => listSenderProfiles(),
  );
  const [selected, setSelected] = useState<string | null>(null);

  // Default selection: first brand, else create mode.
  const effective = selected ?? (profiles && profiles.length > 0 ? profiles[0].id : NEW);

  async function handleDelete(id: string) {
    if (!confirm("Delete this brand? Markets using it will fall back to no brand.")) return;
    try {
      await deleteSenderProfile(id);
      setSelected(null);
      mutate();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-2xl font-bold">Brands</h1>
        <p className="text-sm text-muted-foreground">
          Each brand is a sender profile — its own company details, tone, signature, and catalog.
          Assign a brand to a market so that market sends as it.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-[240px_1fr] gap-4">
        <div className="space-y-1">
          {isLoading ? (
            <div className="flex items-center gap-2 text-muted-foreground p-2">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading…
            </div>
          ) : (
            (profiles ?? []).map((p) => (
              <div key={p.id} className="group relative">
                <button
                  onClick={() => setSelected(p.id)}
                  className={
                    "w-full text-left rounded-md border px-3 py-2 pr-8 text-sm transition " +
                    (effective === p.id
                      ? "border-blue-400 bg-blue-50"
                      : "hover:border-blue-300")
                  }
                >
                  <div className="font-medium truncate">{p.name || "Untitled"}</div>
                  <div className="text-xs text-muted-foreground truncate">{p.company_name}</div>
                </button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="absolute right-1 top-1.5 h-6 w-6 opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-red-600"
                  onClick={() => handleDelete(p.id)}
                  title="Delete brand"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))
          )}
          <Button
            variant={effective === NEW ? "default" : "outline"}
            size="sm"
            className="w-full mt-1"
            onClick={() => setSelected(NEW)}
          >
            <Plus className="h-4 w-4 mr-1" /> New brand
          </Button>
        </div>

        <div>
          <SenderProfileEditor
            key={effective}
            profileId={effective === NEW ? null : effective}
            onSaved={() => mutate()}
          />
        </div>
      </div>
    </div>
  );
}
