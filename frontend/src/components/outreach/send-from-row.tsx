"use client";

import { useState } from "react";
import useSWR from "swr";
import { listChannels, setGroupChannel } from "@/lib/api/outreach";
import type { UserChannel } from "@/lib/types/outreach";

// SendFromRow shows (and, before launch, lets you change) the email account a
// group sends from. Once launched the account is locked, so it renders read-only.
export function SendFromRow({
  groupId,
  channelId,
  senderEmail,
  editable,
  onChanged,
}: {
  groupId: string;
  channelId: string | null;
  senderEmail: string;
  editable: boolean;
  onChanged: () => void;
}) {
  const { data } = useSWR<UserChannel[]>(editable ? "/outreach/channels" : null, () => listChannels());
  const channels = (data ?? []).filter((c) => c.enabled);
  const [saving, setSaving] = useState(false);

  async function change(id: string) {
    if (!id || id === channelId) return;
    setSaving(true);
    try {
      await setGroupChannel(groupId, id);
      onChanged();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Couldn't change the sending account");
    } finally {
      setSaving(false);
    }
  }

  if (!editable) {
    return (
      <span className="text-xs text-muted-foreground">
        Sends from <span className="font-medium text-foreground">{senderEmail || "—"}</span>
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
      Sends from
      <select
        value={channelId ?? ""}
        onChange={(e) => change(e.target.value)}
        disabled={saving || channels.length === 0}
        className="h-7 rounded border border-input bg-background px-2 text-xs"
      >
        {channelId == null && <option value="">— pick an account —</option>}
        {channels.map((c) => (
          <option key={c.id} value={c.id}>
            {c.display_label || c.from_email}
          </option>
        ))}
      </select>
    </span>
  );
}
