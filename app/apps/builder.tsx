import { useState } from "react";
import { Button, Textarea, Status, json } from "../shared";

type BuiltApp = {
  slug: string;
  name: string;
  updated_at?: string;
  html?: string;
  description?: string;
  public?: boolean;
};
export function AppBuilder({
  app,
  onSaved,
}: {
  app?: BuiltApp;
  onSaved?: (app: BuiltApp) => void;
}) {
  const [current, setCurrent] = useState(app);
  const [instruction, setInstruction] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const [turns, setTurns] = useState<string[]>([]);
  return (
    <section className="space-y-4" aria-label="Build with Micro">
      {current && (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="font-semibold">{current.name}</h2>
          <div className="flex gap-2">
            <Button asChild>
              <a href={`/apps/${encodeURIComponent(current.slug)}`}>Open</a>
            </Button>
            <Button asChild>
              <a href={`/apps/${encodeURIComponent(current.slug)}/edit`}>
                Editor
              </a>
            </Button>
          </div>
        </div>
      )}
      {current && (
        <iframe
          key={revision}
          title={`${current.name} preview`}
          src={`/apps/${encodeURIComponent(current.slug)}`}
          className="h-[50dvh] min-h-64 w-full rounded-lg border bg-background"
        />
      )}
      {!!turns.length && (
        <details className="text-sm">
          <summary className="cursor-pointer text-muted-foreground">
            Build history ({turns.length})
          </summary>
          <ol className="mt-2 space-y-2">
            {turns.map((turn, i) => (
              <li key={i}>{turn}</li>
            ))}
          </ol>
        </details>
      )}
      <form
        className="space-y-3"
        onSubmit={async (e) => {
          e.preventDefault();
          if (!instruction.trim() || busy) return;
          setBusy(true);
          setError("");
          try {
            const body = new URLSearchParams(
              current ? { instruction } : { description: instruction },
            );
            const saved = await json<BuiltApp>(
              current
                ? `/apps/${encodeURIComponent(current.slug)}/ai-edit`
                : "/apps/generate",
              { method: "POST", body },
            );
            setCurrent(saved);
            onSaved?.(saved);
            setRevision((v) => v + 1);
            setTurns((v) => [...v, instruction]);
            setInstruction("");
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <label className="block space-y-2 font-medium">
          <span>
            {current
              ? "What should Micro change?"
              : "What would you like to build?"}
          </span>
          <Textarea
            rows={3}
            required
            disabled={busy}
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            placeholder={
              current
                ? "Describe the next change…"
                : "Describe an app and Micro will build it…"
            }
          />
        </label>
        <div className="flex items-center gap-3">
          <Button disabled={busy || !instruction.trim()}>
            {busy
              ? current
                ? "Updating…"
                : "Building…"
              : current
                ? "Apply changes"
                : "Build with Micro"}
          </Button>
          <span className="text-xs text-muted-foreground">
            Uses credits · changes save automatically
          </span>
        </div>
        {busy && (
          <p role="status" className="text-sm text-muted-foreground">
            Micro is working. Keep this page open; the preview will appear here.
          </p>
        )}
        {error && <Status error>{error}</Status>}
      </form>
    </section>
  );
}
