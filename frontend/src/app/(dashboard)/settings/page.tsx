"use client";

import { useState } from "react";
import { useAuth } from "@/lib/providers/auth-provider";
import { updateMe } from "@/lib/api/auth";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { CountryFlag } from "@/components/shared/country-flag";
import { COUNTRY_LIST } from "@/lib/utils/country";
import { Loader2, Check } from "lucide-react";

export default function SettingsPage() {
  const { user } = useAuth();
  const [editing, setEditing] = useState(false);
  const [companyName, setCompanyName] = useState(user?.company_name || "");
  const [homeCountry, setHomeCountry] = useState(user?.home_country || "");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");

  if (!user) return null;

  async function handleSave() {
    setSaving(true);
    setError("");
    try {
      await updateMe({
        company_name: companyName || undefined,
        home_country: homeCountry || undefined,
      });
      setSaved(true);
      setEditing(false);
      setTimeout(() => setSaved(false), 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-6 max-w-2xl">
      <h1 className="text-2xl font-bold">Settings</h1>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">Profile</CardTitle>
          {!editing ? (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              Edit
            </Button>
          ) : (
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={() => setEditing(false)}>
                Cancel
              </Button>
              <Button size="sm" onClick={handleSave} disabled={saving}>
                {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : "Save"}
              </Button>
            </div>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          {error && <p className="text-sm text-red-600">{error}</p>}
          {saved && (
            <p className="text-sm text-emerald-600 flex items-center gap-1">
              <Check className="h-4 w-4" /> Saved
            </p>
          )}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Email</p>
              <p className="font-medium">{user.email}</p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Company</p>
              {editing ? (
                <Input
                  value={companyName}
                  onChange={(e) => setCompanyName(e.target.value)}
                  className="mt-1"
                />
              ) : (
                <p className="font-medium">{companyName || "-"}</p>
              )}
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Home Country</p>
              {editing ? (
                <Select value={homeCountry} onValueChange={(v) => setHomeCountry(v ?? "")}>
                  <SelectTrigger className="mt-1">
                    <SelectValue placeholder="Select country" />
                  </SelectTrigger>
                  <SelectContent>
                    {COUNTRY_LIST.map((c) => (
                      <SelectItem key={c.code} value={c.code}>
                        {c.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <p className="font-medium">
                  {homeCountry ? <CountryFlag code={homeCountry} /> : "-"}
                </p>
              )}
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Plan</p>
              <Badge variant="outline">{user.subscription_tier}</Badge>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">API Usage</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Remaining API calls</p>
          <p className="text-3xl font-bold font-mono">{user.api_calls_remaining}</p>
        </CardContent>
      </Card>
    </div>
  );
}
