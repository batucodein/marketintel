"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { detectEmailSettings, connectEmail, type EmailSettings } from "@/lib/api/outreach";
import { Loader2, Mail, ChevronDown, ChevronRight } from "lucide-react";

// ConnectEmailForm connects any mailbox over IMAP/SMTP (own domain or any
// provider). It autodiscovers server settings from the email, tests the
// connection, then stores the credentials encrypted. For Gmail/Outlook it steers
// the user to OAuth instead (those need an app password for IMAP).
export function ConnectEmailForm({ onConnected }: { onConnected: () => void }) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [advanced, setAdvanced] = useState(false);
  const [settings, setSettings] = useState<EmailSettings | null>(null);
  const [username, setUsername] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [ok, setOk] = useState<string | null>(null);

  async function detect() {
    const e = email.trim();
    if (!e || !e.includes("@")) return;
    try {
      setSettings(await detectEmailSettings(e));
    } catch {
      /* best-effort prefill */
    }
  }

  function update<K extends keyof EmailSettings>(k: K, v: EmailSettings[K]) {
    setSettings((s) => (s ? { ...s, [k]: v } : s));
  }

  async function submit() {
    setError(null);
    setOk(null);
    const e = email.trim();
    if (!e || !password) {
      setError("Email and password are required.");
      return;
    }
    setBusy(true);
    try {
      const res = await connectEmail({
        email: e,
        password,
        username: username.trim() || undefined,
        imap_host: settings?.imap_host,
        imap_port: settings?.imap_port,
        smtp_host: settings?.smtp_host,
        smtp_port: settings?.smtp_port,
        security: settings?.security,
      });
      setOk(res.reconnected ? "Reconnected — credentials updated." : "Mailbox connected.");
      setPassword("");
      onConnected();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Connection failed.");
    } finally {
      setBusy(false);
    }
  }

  const providerHint = settings?.provider_hint;

  return (
    <div className="space-y-3">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {ok && (
        <Alert>
          <AlertDescription>{ok}</AlertDescription>
        </Alert>
      )}
      {providerHint === "gmail" && (
        <Alert>
          <AlertDescription>
            This mailbox is on <span className="font-medium">Google / Workspace</span>. Two things to
            do first: in Gmail settings enable <span className="font-medium">IMAP</span>, and create
            an <span className="font-medium">app password</span> (myaccount.google.com → Security →
            App passwords — needs 2-step verification on). Use that app password below, not your
            normal one.
          </AlertDescription>
        </Alert>
      )}
      {providerHint === "outlook" && (
        <Alert>
          <AlertDescription>
            This mailbox is on <span className="font-medium">Microsoft 365 / Outlook</span>. Many
            organizations <span className="font-medium">disable IMAP/SMTP</span> — if the connection
            fails, ask your admin to enable IMAP and SMTP AUTH for the mailbox, and use an app
            password.
          </AlertDescription>
        </Alert>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <div className="space-y-1">
          <Label>Email address</Label>
          <Input
            type="email"
            placeholder="you@yourdomain.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            onBlur={detect}
          />
        </div>
        <div className="space-y-1">
          <Label>Password (or app password)</Label>
          <Input
            type="password"
            placeholder="••••••••"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
      </div>

      <button
        type="button"
        onClick={() => setAdvanced((v) => !v)}
        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
      >
        {advanced ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        Advanced (server settings){settings ? " — autodetected" : ""}
      </button>

      {advanced && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 rounded-md border bg-muted/20 p-3">
          <div className="space-y-1">
            <Label className="text-xs">IMAP host</Label>
            <Input className="h-8 text-sm" value={settings?.imap_host ?? ""} onChange={(e) => update("imap_host", e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">IMAP port</Label>
            <Input className="h-8 text-sm" type="number" value={settings?.imap_port ?? 993} onChange={(e) => update("imap_port", Number(e.target.value))} />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">SMTP host</Label>
            <Input className="h-8 text-sm" value={settings?.smtp_host ?? ""} onChange={(e) => update("smtp_host", e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">SMTP port</Label>
            <Input className="h-8 text-sm" type="number" value={settings?.smtp_port ?? 465} onChange={(e) => update("smtp_port", Number(e.target.value))} />
          </div>
          <div className="space-y-1">
            <Label className="text-xs">Security</Label>
            <select
              className="w-full h-8 rounded-md border border-input bg-background px-2 text-sm"
              value={settings?.security ?? "tls"}
              onChange={(e) => update("security", e.target.value)}
            >
              <option value="tls">SSL/TLS (465/993)</option>
              <option value="starttls">STARTTLS (587)</option>
            </select>
          </div>
          <div className="space-y-1 col-span-2 md:col-span-1">
            <Label className="text-xs">Username (if not email)</Label>
            <Input className="h-8 text-sm" value={username} onChange={(e) => setUsername(e.target.value)} placeholder={email || "you@domain.com"} />
          </div>
        </div>
      )}

      <Button onClick={submit} disabled={busy}>
        {busy ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Mail className="h-4 w-4 mr-2" />}
        {busy ? "Testing connection…" : "Connect mailbox"}
      </Button>
      <p className="text-[11px] text-muted-foreground">
        We test the connection before saving, store your credentials encrypted, and never share them
        — your email stays in your account.
      </p>
    </div>
  );
}
