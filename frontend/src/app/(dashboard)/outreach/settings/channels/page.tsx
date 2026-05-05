"use client";

import { useState, useEffect } from "react";
import { useSearchParams } from "next/navigation";
import useSWR from "swr";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { listChannels, deleteChannel, setDefaultChannel, getGmailAuthURL } from "@/lib/api/outreach";
import type { UserChannel } from "@/lib/types/outreach";
import { Loader2, Mail, Trash2, Star } from "lucide-react";

export default function ChannelsPage() {
  const searchParams = useSearchParams();
  const { data, mutate, isLoading } = useSWR<UserChannel[]>("/outreach/channels", () =>
    listChannels(),
  );
  const [connecting, setConnecting] = useState(false);
  const [banner, setBanner] = useState<{ kind: "ok" | "error"; text: string } | null>(null);

  useEffect(() => {
    if (searchParams.get("connected") === "1") {
      setBanner({ kind: "ok", text: "Gmail account connected." });
    } else {
      const err = searchParams.get("error");
      if (err) setBanner({ kind: "error", text: "Connection failed: " + err });
    }
  }, [searchParams]);

  async function handleConnect() {
    setConnecting(true);
    try {
      const { auth_url } = await getGmailAuthURL();
      window.location.href = auth_url;
    } catch (e) {
      setBanner({ kind: "error", text: e instanceof Error ? e.message : "Failed to start OAuth" });
      setConnecting(false);
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Disconnect this channel? Conversation history stays in your inbox; you can reconnect later by clicking Connect Gmail with the same account.")) return;
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
          Connect a Gmail account to send and receive emails from MarketIntel. More channels (Outlook, WhatsApp) will follow.
        </p>
      </div>

      {banner && (
        <Alert variant={banner.kind === "error" ? "destructive" : "default"}>
          <AlertDescription>{banner.text}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">Connected accounts</CardTitle>
            <Button onClick={handleConnect} disabled={connecting}>
              {connecting ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin mr-2" /> Redirecting…
                </>
              ) : (
                <>
                  <Mail className="h-4 w-4 mr-2" /> Connect Gmail
                </>
              )}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading…
            </div>
          ) : !data || data.length === 0 ? (
            <div className="text-sm text-muted-foreground py-8 text-center">
              No channels connected yet. Click <b>Connect Gmail</b> to link your first account.
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
                    {!ch.enabled ? (
                      <Button size="sm" variant="outline" onClick={handleConnect}>
                        Reconnect
                      </Button>
                    ) : (
                      <>
                        {!ch.is_default && (
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
                      </>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
