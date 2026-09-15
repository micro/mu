import { useEffect, useState } from "react";
import { Switch } from "./ui/switch";
import { Button } from "./ui/button";
import { PageHeading, Status } from "./layout";
import { json } from "../lib/api";
type Preferences = {
  enabled: boolean;
  include_world_news: boolean;
  timezone: string;
  time: string;
  title?: string;
  repeat?: string;
};
export function InboxSettings() {
  const [prefs, setPrefs] = useState<Preferences>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [saved, setSaved] = useState(false);
  useEffect(() => {
    json<Preferences>("/inbox/settings")
      .then(setPrefs)
      .catch((e) => setError(e.message));
  }, []);
  async function update(key: "enabled" | "include_world_news", value: boolean) {
    if (!prefs) return;
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      const next = await json<Preferences>("/inbox/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          ...prefs,
          [key]: value,
          timezone:
            prefs.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone,
        }),
      });
      setPrefs(next);
      setSaved(true);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="max-w-2xl">
      <PageHeading
        title="Inbox settings"
        actions={
          <Button asChild variant="outline">
            <a href="/inbox">Inbox</a>
          </Button>
        }
      />
      <p className="mb-6 text-muted-foreground">
        Choose the scheduled messages Micro sends you.
      </p>
      {error && <Status error>{error}</Status>}
      {prefs ? (
        <div className="divide-y rounded-xl border px-4 sm:px-5">
          <div className="flex items-start justify-between gap-5 py-5">
            <div>
              <label htmlFor="morning-brief" className="font-medium">
                {prefs.title || "Morning brief"}
              </label>
              <p
                id="brief-description"
                className="mt-1 text-sm text-muted-foreground"
              >
                A summary of your day, delivered to your inbox
                {prefs.repeat === "weekdays" ? " on weekdays" : " daily"} at{" "}
                {prefs.time}
                {prefs.timezone
                  ? " (" + prefs.timezone + ")"
                  : " in your local timezone"}
                .
              </p>
            </div>
            <Switch
              id="morning-brief"
              aria-describedby="brief-description"
              checked={prefs.enabled}
              disabled={busy}
              onCheckedChange={(value) => update("enabled", value)}
            />
          </div>
          <div className="flex items-start justify-between gap-5 py-5">
            <div>
              <label htmlFor="world-news" className="font-medium">
                Include world news
              </label>
              <p
                id="news-description"
                className="mt-1 text-sm text-muted-foreground"
              >
                Add a short world news section to your brief.
              </p>
            </div>
            <Switch
              id="world-news"
              aria-describedby="news-description"
              checked={prefs.include_world_news}
              disabled={busy || !prefs.enabled}
              onCheckedChange={(value) => update("include_world_news", value)}
            />
          </div>
        </div>
      ) : (
        !error && <Status>Loading settings…</Status>
      )}
      <div className="mt-3 min-h-6">
        <Status>{busy ? "Saving…" : saved ? "Saved." : ""}</Status>
      </div>
    </div>
  );
}
