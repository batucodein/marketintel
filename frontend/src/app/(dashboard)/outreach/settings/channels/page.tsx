"use client";

import { useState } from "react";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { listChannels, deleteChannel, setDefaultChannel } from "@/lib/api/outreach";
import type { UserChannel } from "@/lib/types/outreach";
import { ConnectEmailForm } from "@/components/outreach/connect-email-form";
import { Loader2, Mail, Trash2, Star } from "lucide-react";

export default function ChannelsPage() {
  const { data, mutate, isLoading } = useSWR<UserChannel[]>("/outreach/channels", () =>
    listChannels(),
  );

  async function handleDelete(id: string) {
    if (!confirm("Disconnect this mailbox? Conversation history stays in your account; you can reconnect it any time below.")) return;
    await deleteChannel(id);
    mutate();
  }

  async function handleSetDefault(id: string) {
    await setDefaultChannel(id);
    mutate();
  }

  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <h1 className="text-2xl font-bold">Email channels</h1>
        <p className="text-sm text-muted-foreground">
          Connect any mailbox — your own domain, Gmail/Workspace, Outlook, Zoho, and more — over
          IMAP/SMTP. Credentials are stored encrypted and your email never leaves your account.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Connected accounts</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading…
            </div>
          ) : !data || data.length === 0 ? (
            <div className="text-sm text-muted-foreground py-8 text-center">
              No mailbox connected yet. Use the form below to connect your first account.
            </div>
          ) : (
            <div className="space-y-2">
              {data.map((ch) => (
                <div
                  key={ch.id}
                  className={`flex items-center justify-between border rounded-md p-3 ${ch.enabled ? "" : "opacity-60"}`}
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <Mail className={`h-5 w-5 shrink-0 ${ch.enabled ? "text-blue-700" : "text-muted-foreground"}`} />
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium truncate">{ch.display_label}</span>
                        {ch.is_default && (
                          <Badge variant="default" className="text-xs">
                            Default
                          </Badge>
                        )}
                        {!ch.enabled && (
                          <Badge variant="outline" className="text-xs">
                            Disconnected
                          </Badge>
                        )}
                      </div>
                      <div className="text-xs text-muted-foreground font-mono">{ch.from_email}</div>
                    </div>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    {!ch.enabled && (
                      <span className="text-[11px] text-muted-foreground mr-1">
                        reconnect below
                      </span>
                    )}
                    {ch.enabled && !ch.is_default && (
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => handleSetDefault(ch.id)}
                        title="Make default"
                      >
                        <Star className="h-4 w-4" />
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => handleDelete(ch.id)}
                      title="Disconnect (keeps history)"
                    >
                      <Trash2 className="h-4 w-4 text-red-600" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Connect another email account</CardTitle>
          <p className="text-xs text-muted-foreground">
            Your own domain or any provider, over IMAP/SMTP. We test the connection before saving.
          </p>
        </CardHeader>
        <CardContent>
          <ConnectEmailForm onConnected={() => mutate()} />
        </CardContent>
      </Card>
    </div>
  );
}
