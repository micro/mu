import { useState } from "react";
import {
  useData,
  json,
  call,
  Button,
  Input,
  Status,
  Form,
  Search,
  Action,
  Link,
  When,
} from "./shared";
import { PageHeading } from "../web/src/components/layout";
export function Images() {
  const [scope, setScope] = useState("public"),
    [q, setQ] = useState(""),
    [cursor, setCursor] = useState(""),
    [creating, setCreating] = useState(false),
    [editing, setEditing] = useState<any>();
  const { data, error, refresh } = useData<any>(async () => {
    if (scope === "public" && !q && !cursor) {
      const d = await json<any>("/images");
      return { items: [d.daily, ...(d.stock || [])].filter(Boolean) };
    }
    return call("images", q ? "search" : "list", {
      scope,
      query: q,
      cursor,
      limit: 20,
    });
  }, [scope, q, cursor]);
  return (
    <>
      <PageHeading
        title="Images"
        actions={<Button onClick={() => setCreating(!creating)}>New</Button>}
      />
      <div className="mb-4 flex flex-wrap gap-2">
        {["public", "mine", "all", "daily"].map((s) => (
          <Button
            key={s}
            aria-pressed={s === scope}
            onClick={() => {
              setScope(s);
              setCursor("");
            }}
          >
            {s[0].toUpperCase() + s.slice(1)}
          </Button>
        ))}
      </div>
      <ImageUpload
        done={async () => {
          setScope("mine");
          await refresh();
        }}
      />
      <Search
        placeholder="Find an image"
        onSearch={(v) => {
          setQ(v);
          setCursor("");
        }}
      />
      {creating && (
        <div className="mb-6">
          <Form
            fields={[
              {
                name: "prompt",
                label: "Describe your image",
                type: "textarea",
                required: true,
              },
            ]}
            label="Generate"
            submit={async (v) => {
              await call("images", "generate", v);
              setCreating(false);
              setScope("mine");
              await refresh();
            }}
          />
        </div>
      )}
      {editing && (
        <div className="mb-6">
          <Form
            key={editing.id}
            initial={{ ...editing, tags: (editing.tags || []).join(", ") }}
            fields={[
              { name: "title", label: "Title" },
              { name: "description", label: "Description", type: "textarea" },
              { name: "tags", label: "Tags" },
            ]}
            submit={async (v) => {
              await call("images", "update", {
                ...v,
                id: editing.id,
                tags: v.tags
                  .split(",")
                  .map((s) => s.trim())
                  .filter(Boolean),
              });
              setEditing(undefined);
              await refresh();
            }}
          />
          <Button className="mt-2" onClick={() => setEditing(undefined)}>
            Cancel
          </Button>
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <div className="grid gap-6 sm:grid-cols-2 xl:grid-cols-3">
        {data?.items?.map((im: any, i: number) => (
          <figure key={im.id || im.url || i} className="min-w-0 space-y-3">
            <a href={im.url}>
              <img
                className="aspect-square w-full rounded object-cover"
                alt={im.title || im.prompt || im.description || ""}
                src={im.url}
                loading="lazy"
              />
            </a>
            <figcaption className="space-y-2">
              <h2 className="font-medium">{im.title || im.theme}</h2>
              <p className="break-words text-sm text-muted-foreground">
                {im.description || im.prompt}
              </p>
              <When value={im.created_at || im.date} />
              <div className="flex flex-wrap gap-2">
                <Link url={im.url}>Open</Link>
                {scope === "mine" && (
                  <>
                    <Button onClick={() => setEditing(im)}>Edit</Button>
                    <Action
                      run={async () => {
                        await call("images", "share", {
                          id: im.id,
                          public: im.visibility !== "public",
                        });
                        await refresh();
                      }}
                    >
                      {im.visibility === "public" ? "Make private" : "Share"}
                    </Action>
                    <Action
                      danger
                      run={async () => {
                        await call("images", "delete", { id: im.id });
                        await refresh();
                      }}
                    >
                      Delete
                    </Action>
                  </>
                )}
              </div>
            </figcaption>
          </figure>
        ))}
      </div>
      {data?.items?.length === 0 && (
        <p className="py-6 text-muted-foreground">No images found.</p>
      )}
      {data?.next_cursor && (
        <Button className="mt-6" onClick={() => setCursor(data.next_cursor)}>
          Next
        </Button>
      )}
    </>
  );
}

function ImageUpload({ done }: { done: () => Promise<unknown> }) {
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <details className="mb-5">
      <summary>Upload an image</summary>
      <form
        className="mt-3 space-y-3"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            await json("/images?upload=1", {
              method: "POST",
              body: new FormData(e.currentTarget),
            });
            await done();
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <Input
          type="file"
          name="file"
          accept="image/png,image/jpeg,image/gif"
          required
          aria-label="Image to upload"
        />
        <Input
          name="caption"
          placeholder="Caption"
          aria-label="Caption"
          maxLength={1000}
        />
        <p className="text-sm text-muted-foreground">
          Private. PNG, JPEG or GIF, up to 8 MB and 8 megapixels. GIF uploads
          keep the first frame.
        </p>
        <Button disabled={busy}>{busy ? "Uploading…" : "Upload"}</Button>
        {error && <Status error>{error}</Status>}
      </form>
    </details>
  );
}
