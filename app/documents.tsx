import { useState, useRef } from "react";
import {
  useData,
  json,
  mutate,
  call,
  Button,
  Input,
  Textarea,
  Status,
  Search,
  Rows,
  When,
  Read,
  Action,
  Link,
} from "./shared";
import { PageHeading } from "../web/src/components/layout";
export function Documents({ notes = false }: { notes?: boolean }) {
  const params = new URLSearchParams(location.search),
    base = notes ? "/notes" : "/docs";
  const [query, setQuery] = useState("");
  const { data, error, refresh } = useData<any>(
    () =>
      notes
        ? json(base)
        : json(base, {
            method: "POST",
            body: new URLSearchParams({ action: "search", q: query }),
          }),
    [query, notes],
  );
  const entries = (notes ? data?.notes : data?.docs)?.map((d: any) =>
    notes ? { ...d, title: d.key, content: d.value, updated: d.updated_at } : d,
  );
  const selected = entries?.find(
    (d: any) =>
      d.id === params.get("id") || (notes && d.title === params.get("note")),
  );
  const editing =
    params.has("new") || params.has("edit") || (notes && !!selected);
  return (
    <>
      <PageHeading
        title={notes ? "Notes" : "Documents"}
        actions={
          <>
            <Button asChild>
              <a href={base + "?new=1"}>New</a>
            </Button>
            {!notes && (
              <Button asChild>
                <a href="/docs?import=1">Import</a>
              </Button>
            )}
          </>
        }
      />
      {error && <Status error>{error}</Status>}
      {params.has("import") && !notes ? (
        <ImportDocument />
      ) : editing ? (
        params.has("new") || selected ? (
          <Editor key={selected?.id || "new"} doc={selected} notes={notes} />
        ) : (
          <Status>Loading document…</Status>
        )
      ) : selected ? (
        <>
          <div className="mb-4 flex flex-wrap gap-2">
            <Button asChild>
              <a href={base}>All documents</a>
            </Button>
            <Button asChild>
              <a href={`${base}?id=${encodeURIComponent(selected.id)}&edit=1`}>
                Edit
              </a>
            </Button>
            <Action
              danger
              run={async () => {
                await call("docs", "delete", { id: selected.id });
                location.assign(base);
              }}
            >
              Delete
            </Action>
          </div>
          <h1 className="mb-4 break-words text-2xl font-semibold">
            {selected.title}
          </h1>
          <Read>{selected.content}</Read>
          <p className="mt-5 text-sm text-muted-foreground">
            <When value={selected.updated} />
          </p>
        </>
      ) : (
        <>
          <Search
            onSearch={setQuery}
            placeholder={notes ? "Find a note" : "Search your documents"}
          />
          <Rows
            items={entries?.filter(
              (d: any) =>
                !notes ||
                (d.title + " " + d.content)
                  .toLowerCase()
                  .includes(query.toLowerCase()),
            )}
            render={(d) => (
              <>
                <h2 className="font-medium">
                  <Link url={`${base}?id=${encodeURIComponent(d.id)}`}>
                    {d.title}
                  </Link>
                </h2>
                <p className="line-clamp-2 break-words text-muted-foreground">
                  {d.content}
                </p>
                <p className="text-sm text-muted-foreground">
                  <When value={d.updated} />
                </p>
              </>
            )}
          />
        </>
      )}
    </>
  );
}
function Editor({ doc, notes }: { doc?: any; notes: boolean }) {
  const [title, setTitle] = useState(doc?.title || ""),
    [content, setContent] = useState(doc?.content || ""),
    [pub, setPub] = useState(!!doc?.public),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const body = useRef<HTMLTextAreaElement>(null);
  function insert(before: string, after = "") {
    const el = body.current;
    if (!el) return;
    const a = el.selectionStart,
      b = el.selectionEnd;
    setContent(
      content.slice(0, a) +
        before +
        content.slice(a, b) +
        after +
        content.slice(b),
    );
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(a + before.length, b + before.length);
    });
  }
  async function save() {
    setBusy(true);
    setError("");
    try {
      if (notes) {
        await mutate("/notes", { save: "1", title, text: content });
        location.assign("/notes");
      } else {
        const d = await call("docs", "write", {
          id: doc?.id,
          title,
          content,
          public: pub,
        });
        location.assign("/docs?id=" + encodeURIComponent(d.doc.id));
      }
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Button asChild>
          <a href={notes ? "/notes" : "/docs"}>Back</a>
        </Button>
        <Button disabled={busy} type="submit">
          {busy ? "Saving…" : "Save"}
        </Button>
        {notes && doc && (
          <Action
            danger
            run={async () => {
              await mutate("/notes", { delete: title });
              location.assign("/notes");
            }}
          >
            Delete
          </Action>
        )}
      </div>
      {!notes && (
        <div className="flex flex-wrap gap-2" aria-label="Formatting">
          <Button type="button" onClick={() => insert("**", "**")}>
            Bold
          </Button>
          <Button type="button" onClick={() => insert("*", "*")}>
            Italic
          </Button>
          <Button type="button" onClick={() => insert("## ")}>
            Heading
          </Button>
          <Button type="button" onClick={() => insert("- ")}>
            List
          </Button>
          <Button type="button" onClick={() => insert("[", "](https://)")}>
            Link
          </Button>
        </div>
      )}
      <Input
        aria-label="Title"
        placeholder="Title"
        required
        maxLength={notes ? 40 : 200}
        value={title}
        readOnly={notes && !!doc}
        onChange={(e) => setTitle(e.target.value)}
      />
      <Textarea
        ref={body}
        className="min-h-[45dvh]"
        aria-label="Body"
        placeholder="Write it down"
        rows={notes ? 14 : 24}
        maxLength={notes ? 2000 : 200000}
        required
        value={content}
        onChange={(e) => setContent(e.target.value)}
      />
      {!notes && (
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={pub}
            onChange={(e) => setPub(e.target.checked)}
          />
          Anyone with the link can read it
        </label>
      )}
      {doc?.source_thread && (
        <Link url={"/inbox?id=" + encodeURIComponent(doc.source_thread)}>
          Source conversation
        </Link>
      )}
      {error && <Status error>{error}</Status>}
    </form>
  );
}
function ImportDocument() {
  const [draft, setDraft] = useState<any>(),
    [error, setError] = useState("");
  if (draft) return <Editor doc={draft} notes={false} />;
  return (
    <div className="space-y-4">
      <Button asChild>
        <a href="/docs">Documents</a>
      </Button>
      <label className="block space-y-2">
        <span>Choose a Markdown or plain-text file</span>
        <Input
          type="file"
          accept=".txt,.md,.markdown,text/plain,text/markdown"
          onChange={async (e) => {
            try {
              const file = e.target.files?.[0];
              if (!file) return;
              if (
                file.size > 200000 ||
                !/\.(txt|md|markdown)$/i.test(file.name)
              )
                throw Error("Choose a text or Markdown file up to 200 KB.");
              const text = new TextDecoder("utf-8", { fatal: true }).decode(
                await file.arrayBuffer(),
              );
              if (text.includes("\0")) throw Error("Choose a UTF-8 text file.");
              setDraft({
                title: file.name.replace(/\.[^.]+$/, ""),
                content: text,
              });
            } catch (e) {
              setError((e as Error).message);
            }
          }}
        />
      </label>
      <p className="text-sm text-muted-foreground">
        Review the draft before saving.
      </p>
      {error && <Status error>{error}</Status>}
    </div>
  );
}
