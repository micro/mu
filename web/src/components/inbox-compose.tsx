import { useEffect, useState, type FormEvent } from "react";
import { Field } from "./ui/field";
import { Label } from "./ui/label";
import { Textarea } from "./ui/textarea";
import { Button } from "./ui/button";
import { PageHeading, Status } from "./layout";
import { initialData, json, mutate } from "../lib/api";
export function InboxCompose() {
  const [values, setValues] = useState<{
      to: string;
      subject: string;
      body: string;
      on: string;
    } | undefined>(() => initialData()),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    json<any>("/inbox/new" + location.search)
      .then(setValues)
      .catch((e) => setError(e.message));
  }, []);
  async function send(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const body = Object.fromEntries(new FormData(e.currentTarget)) as Record<
      string,
      string
    >;
    try {
      await mutate("/inbox/new", body);
      location.assign(
        values?.on ? "/inbox?id=" + encodeURIComponent(values.on) : "/inbox",
      );
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  }
  return (
    <div className="max-w-3xl">
      <PageHeading
        title="New message"
        actions={
          <Button asChild variant="outline">
            <a href="/inbox">Inbox</a>
          </Button>
        }
      />
      {error && <Status error>{error}</Status>}
      {values ? (
        <form className="space-y-4" onSubmit={send}>
          <input type="hidden" name="on" value={values.on} />
          <Field
            label="To"
            name="to"
            defaultValue={values.to}
            placeholder="@username or email address"
            required
          />
          <Field label="Subject" name="subject" defaultValue={values.subject} />
          <div className="space-y-2">
            <Label htmlFor="message">Message</Label>
            <Textarea
              id="message"
              name="body"
              defaultValue={values.body}
              className="min-h-56"
              maxLength={40000}
              required
            />
          </div>
          <Button disabled={busy}>{busy ? "Sending…" : "Send"}</Button>
        </form>
      ) : (
        !error && <Status>Loading…</Status>
      )}
    </div>
  );
}
