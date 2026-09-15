import { useEffect, useState, type FormEvent } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Settings, Plus } from "lucide-react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Textarea } from "./ui/textarea";
import { Label } from "./ui/label";
import { PageHeading, Pager, Status } from "./layout";
import { Email } from "./email";
import { ResultView } from "./result";
import { json, mutate, type Result } from "../lib/api";
type Row = {
  id: string;
  sender: string;
  subject: string;
  kind: string;
  preview: string;
  updated: string;
  unread: boolean;
};
type Data = {
  items: Row[];
  page: number;
  total: number;
  page_size: number;
  thread?: Row;
  messages?: {
    id: string;
    role: string;
    text: string;
    from: string;
    at: string;
    results?: Result[];
    email_html?: string;
  }[];
  reply_to: string;
  previous: string;
  next: string;
  has_older: boolean;
  before: number;
  open: string;
};
export function InboxPage() {
  const [data, setData] = useState<Data>(),
    [error, setError] = useState(""),
    [page, setPage] = useState(
      Number(new URLSearchParams(location.search).get("page")) || 1,
    ),
    [query, setQuery] = useState(""),
    [search, setSearch] = useState(""),
    [reply, setReply] = useState(false),
    [body, setBody] = useState(""),
    [ask, setAsk] = useState(""),
    [busy, setBusy] = useState(false);
  const id = new URLSearchParams(location.search).get("id"),
    base = location.pathname;
  async function load() {
    setError("");
    try {
      setData(
        await json<Data>(
          base +
            "?" +
            new URLSearchParams({ page: String(page), ...(id ? { id } : {}) }),
          search
            ? { method: "POST", body: new URLSearchParams({ q: search }) }
            : {},
        ),
      );
    } catch (e) {
      setError((e as Error).message);
    }
  }
  useEffect(() => {
    load();
  }, [page, search, id]);
  async function act(path: string, values: Record<string, string>) {
    setBusy(true);
    setError("");
    try {
      await mutate(path, values);
      if (
        values.action === "delete" ||
        path.includes("/delete") ||
        values.action === "handled"
      ) {
        location.href = "/inbox";
        return;
      }
      await load();
      setBody("");
      setReply(false);
      setAsk("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function older() {
    setBusy(true);
    try {
      const next = await json<Data>(
        base +
          "?" +
          new URLSearchParams({ id: id!, before: String(data?.before || 0) }),
      );
      setData((previous) =>
        previous
          ? {
              ...previous,
              messages: [
                ...(next.messages || []),
                ...(previous.messages || []),
              ],
              before: next.before,
              has_older: next.has_older,
            }
          : next,
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const thread = data?.thread;
  return (
    <div className="mx-auto max-w-4xl">
      <PageHeading
        title={thread?.subject || "Inbox"}
        actions={
          <>
            <Button asChild variant="outline">
              <a href="/inbox/settings">
                <Settings />
                Settings
              </a>
            </Button>
            {!thread && (
              <Button asChild>
                <a href="/inbox/new">
                  <Plus />
                  New
                </a>
              </Button>
            )}
          </>
        }
      />
      {error && <Status error>{error}</Status>}
      {!data && !error && <Status>Loading inbox…</Status>}
      {thread ? (
        <>
          <nav className="mb-5 flex flex-wrap items-center gap-2">
            <Button asChild variant="outline">
              <a href="/inbox">Inbox</a>
            </Button>
            {[data.previous, data.next].map(
              (to, i) =>
                to && (
                  <Button key={i} asChild variant="ghost">
                    <a href={base + "?id=" + encodeURIComponent(to)}>
                      {i ? "Next" : "Previous"}
                    </a>
                  </Button>
                ),
            )}
            <Button
              variant="outline"
              disabled={busy}
              onClick={() =>
                act(base, {
                  action: "handled",
                  id: thread.id,
                  reviewed: thread.updated,
                })
              }
            >
              Done
            </Button>
            <Button
              variant="ghost"
              disabled={busy}
              onClick={() => {
                if (confirm("Delete this conversation?"))
                  act("/inbox/delete", { id: thread.id });
              }}
            >
              Delete
            </Button>
          </nav>
          <div className="mb-3 flex gap-2">
            {data.has_older && (
              <Button variant="outline" disabled={busy} onClick={older}>
                Earlier messages
              </Button>
            )}
            {data.open && (
              <Button variant="outline" asChild>
                <a href={data.open}>Open</a>
              </Button>
            )}
          </div>
          <div className="divide-y">
            {data.messages?.map((m) => (
              <article className="py-4 first:pt-0" key={m.id}>
                <div className="mb-2 flex items-baseline justify-between gap-3 text-sm text-muted-foreground">
                  <span className="min-w-0 truncate">
                    {m.role === "agent" ? "Micro" : m.from || thread.sender}
                  </span>
                  <time className="shrink-0" dateTime={m.at}>
                    {date(m.at)}
                  </time>
                </div>
                {m.email_html ? (
                  <Email html={m.email_html} />
                ) : m.role === "agent" ? (
                  <div className="markdown">
                    <Markdown remarkPlugins={[remarkGfm]}>{m.text}</Markdown>
                  </div>
                ) : (
                  <p className="whitespace-pre-wrap break-words leading-relaxed">
                    {m.text}
                  </p>
                )}
                {m.results?.map((r, i) => (
                  <ResultView key={i} item={r} signedIn />
                ))}
              </article>
            ))}
          </div>
          {data.reply_to && (
            <div className="mt-4">
              {reply ? (
                <form
                  className="space-y-3"
                  onSubmit={(e) => {
                    e.preventDefault();
                    act("/inbox/new", {
                      inline: "1",
                      on: thread.id,
                      to: data.reply_to,
                      subject: /^re:/i.test(thread.subject)
                        ? thread.subject
                        : "Re: " + thread.subject,
                      body,
                    });
                  }}
                >
                  <Label htmlFor="reply">Reply</Label>
                  <Textarea
                    id="reply"
                    rows={4}
                    value={body}
                    required
                    maxLength={40000}
                    onChange={(e) => setBody(e.target.value)}
                  />
                  <div className="flex gap-2">
                    <Button disabled={busy}>Send</Button>
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => setReply(false)}
                    >
                      Cancel
                    </Button>
                  </div>
                </form>
              ) : (
                <Button onClick={() => setReply(true)}>Reply</Button>
              )}
            </div>
          )}
          <form
            className="mt-6 space-y-2 border-t pt-5"
            onSubmit={(e) => {
              e.preventDefault();
              act(base, { ask, id: thread.id });
            }}
          >
            <Label htmlFor="inbox-ask">Ask Micro about this</Label>
            <div className="flex gap-2">
              <Input
                id="inbox-ask"
                value={ask}
                onChange={(e) => setAsk(e.target.value)}
                required
                placeholder="What would you like done?"
              />
              <Button disabled={busy}>Ask</Button>
            </div>
          </form>
        </>
      ) : (
        data && (
          <>
            <form
              className="mb-4 flex gap-2"
              onSubmit={(e: FormEvent) => {
                e.preventDefault();
                setPage(1);
                setSearch(query);
              }}
            >
              <Input
                aria-label="Search inbox"
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search inbox"
              />
              <Button>Search</Button>
            </form>
            <div className="divide-y border-y">
              {data.items?.map((t) => (
                <a
                  key={t.id}
                  className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1 rounded-sm py-3 hover:bg-accent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                  href={
                    base +
                    "?" +
                    new URLSearchParams({ id: t.id, page: String(page) })
                  }
                >
                  <span
                    className={
                      "truncate " + (t.unread ? "font-semibold" : "font-medium")
                    }
                  >
                    {t.sender || "Micro"}
                  </span>
                  <time
                    className="text-right text-sm text-muted-foreground"
                    title={new Date(t.updated).toLocaleString()}
                    dateTime={t.updated}
                  >
                    {date(t.updated)}
                  </time>
                  <div className="col-span-2 flex min-w-0 items-baseline gap-2">
                    <span className="w-16 shrink-0 text-sm capitalize text-muted-foreground">
                      {t.kind}
                    </span>
                    <span className="truncate">{t.subject || "Untitled"}</span>
                  </div>
                  <p className="col-span-2 truncate text-sm text-muted-foreground">
                    {t.preview}
                  </p>
                </a>
              ))}
              {!data.items?.length && (
                <p className="py-8 text-muted-foreground">
                  Your inbox is empty.
                </p>
              )}
            </div>
            <Pager
              page={data.page}
              total={data.total}
              size={data.page_size}
              onChange={setPage}
            />
          </>
        )
      )}
    </div>
  );
}
function date(value: string) {
  const d = new Date(value);
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}
