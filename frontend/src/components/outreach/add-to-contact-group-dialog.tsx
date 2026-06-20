"use client";

import { useState } from "react";
import useSWR from "swr";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  listContactGroups,
  listSenderProfiles,
  createContactGroup,
  addBusinessesToContactGroup,
} from "@/lib/api/outreach";
import type { ContactGroup, SenderProfile } from "@/lib/types/outreach";
import { Loader2, CheckCircle2 } from "lucide-react";

type Result = { added: { length: number }; already_existed: { length: number }; no_email: { length: number }; added_to_group: number };

// AddToContactGroupDialog: promote a set of market-lead businesses into
// contacts, into a new or existing contact group, with dedup.
export function AddToContactGroupDialog({
  open,
  onOpenChange,
  businessIds,
  defaultBrandId,
  onDone,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  businessIds: string[];
  defaultBrandId: string | null;
  onDone?: () => void;
}) {
  const { data: cgData, mutate: mutateGroups } = useSWR(open ? "/outreach/contact-groups" : null, () =>
    listContactGroups(),
  );
  const { data: profiles } = useSWR<SenderProfile[]>(open ? "/outreach/sender-profile" : null, () =>
    listSenderProfiles(),
  );

  const [mode, setMode] = useState<"new" | "existing">("new");
  const [name, setName] = useState("");
  const [brandId, setBrandId] = useState<string>("");
  const [existingId, setExistingId] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<Result | null>(null);

  const groups = cgData?.groups ?? [];
  // Default the brand to the source market's brand once profiles load.
  const effectiveBrand = brandId || defaultBrandId || "";

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      let res: Result;
      if (mode === "new") {
        if (!name.trim()) {
          setError("Name the contact group.");
          setBusy(false);
          return;
        }
        const out = await createContactGroup({
          name: name.trim(),
          sender_profile_id: effectiveBrand || null,
          business_ids: businessIds,
        });
        res = out.result;
      } else {
        if (!existingId) {
          setError("Pick a contact group.");
          setBusy(false);
          return;
        }
        res = await addBusinessesToContactGroup(existingId, businessIds);
      }
      setResult(res);
      mutateGroups();
      onDone?.();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to add");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full sm:max-w-lg overflow-x-hidden">
        <DialogHeader>
          <DialogTitle>Add {businessIds.length} lead{businessIds.length !== 1 ? "s" : ""} to a contact group</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {result ? (
            <Alert>
              <CheckCircle2 className="h-4 w-4" />
              <AlertDescription>
                <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
                  <span><strong>{result.added_to_group}</strong> added to the group</span>
                  {result.already_existed.length > 0 && (
                    <span className="text-muted-foreground">
                      <strong>{result.already_existed.length}</strong> already a contact
                    </span>
                  )}
                  {result.no_email.length > 0 && (
                    <span className="text-amber-700">
                      <strong>{result.no_email.length}</strong> had no email — fix in the lead drawer
                    </span>
                  )}
                </div>
              </AlertDescription>
            </Alert>
          ) : (
            <>
              <div className="flex gap-2 text-sm">
                <button
                  onClick={() => setMode("new")}
                  className={"rounded-md border px-3 py-1 " + (mode === "new" ? "border-blue-400 bg-blue-50 text-blue-700" : "")}
                >
                  New group
                </button>
                <button
                  onClick={() => setMode("existing")}
                  className={"rounded-md border px-3 py-1 " + (mode === "existing" ? "border-blue-400 bg-blue-50 text-blue-700" : "")}
                  disabled={groups.length === 0}
                >
                  Existing group
                </button>
              </div>

              {mode === "new" ? (
                <>
                  <div className="space-y-1">
                    <Label>Group name</Label>
                    <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="US travertine importers" />
                  </div>
                  <div className="space-y-1">
                    <Label>Brand (sender profile)</Label>
                    <select
                      value={effectiveBrand}
                      onChange={(e) => setBrandId(e.target.value)}
                      className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
                    >
                      <option value="">No brand (set later)</option>
                      {(profiles ?? []).map((p: SenderProfile) => (
                        <option key={p.id} value={p.id}>
                          {p.name || p.company_name}
                        </option>
                      ))}
                    </select>
                    <p className="text-xs text-muted-foreground">
                      Email groups built from this contact group send as this brand.
                    </p>
                  </div>
                </>
              ) : (
                <div className="space-y-1">
                  <Label>Contact group</Label>
                  <select
                    value={existingId}
                    onChange={(e) => setExistingId(e.target.value)}
                    className="w-full h-9 rounded-md border border-input bg-background px-3 text-sm"
                  >
                    <option value="">— Select —</option>
                    {groups.map((g: ContactGroup) => (
                      <option key={g.id} value={g.id}>
                        {g.name} ({g.member_count})
                      </option>
                    ))}
                  </select>
                  <p className="text-xs text-muted-foreground">
                    Already-added contacts are skipped automatically.
                  </p>
                </div>
              )}
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {result ? "Done" : "Cancel"}
          </Button>
          {!result && (
            <Button onClick={submit} disabled={busy}>
              {busy ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
              Add
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
