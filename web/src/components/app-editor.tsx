import { useState } from "react";
import {
  useData,
  json,
  mutate,
  Button,
  Input,
  Textarea,
  Status,
  Form,
  Rows,
  When,
  Action,
} from "../../../apps/shared";
import { PageHeading } from "./layout";
export function AppEditor() {
  if (location.pathname.endsWith("/versions")) return <Versions />;
  return <AppEditorPage />;
}
function AppEditorPage() {
  const creating = location.pathname === "/apps/new",
    slug = location.pathname.split("/")[2];
  const { data, error } = useData<any>(
    () =>
      creating
        ? Promise.resolve({
            name: "",
            description: "",
            html: "",
            tags: "",
            public: false,
            price: 0,
          })
        : json("/apps/" + encodeURIComponent(slug)),
    [slug],
  );
  return (
    <>
      <PageHeading title={creating ? "New app" : "Edit app"} />
      {error && <Status error>{error}</Status>}
      {data && <Editor key={slug} app={data} creating={creating} />}
    </>
  );
}
function Editor({ app, creating }: { app: any; creating: boolean }) {
  const [name, setName] = useState(app.name),
    [description, setDescription] = useState(app.description),
    [html, setHTML] = useState(app.html),
    [pub, setPublic] = useState(!!app.public),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <>
      <form
        className="space-y-4"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            const fields = Object.fromEntries(new FormData(e.currentTarget));
            const saved = await json<any>(
              creating ? "/apps/new" : "/apps/" + encodeURIComponent(app.slug),
              {
                method: creating ? "POST" : "PATCH",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  ...fields,
                  name,
                  description,
                  html,
                  public: pub,
                  price: Number(fields.price || 0),
                }),
              },
            );
            location.assign(
              "/apps/" + encodeURIComponent(saved.slug || app.slug),
            );
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <div className="flex flex-wrap gap-2">
          <Button asChild>
            <a href="/apps">Apps</a>
          </Button>
          <Button disabled={busy} type="submit">
            {busy ? "Saving…" : creating ? "Create" : "Save"}
          </Button>
          {!creating && (
            <Button asChild>
              <a href={"/apps/" + encodeURIComponent(app.slug)}>Open</a>
            </Button>
          )}
          {!creating && (
            <Button asChild>
              <a href={"/apps/" + encodeURIComponent(app.slug) + "/versions"}>
                Versions
              </a>
            </Button>
          )}
        </div>
        <label className="block space-y-2">
          Name
          <Input
            required
            name="name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <label className="block space-y-2">
          Description
          <Textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block space-y-2">
            Tags
            <Input name="tags" defaultValue={app.tags} />
          </label>
          <label className="block space-y-2">
            Price in credits
            <Input
              name="price"
              type="number"
              min={0}
              defaultValue={app.price || 0}
            />
          </label>
        </div>
        <label className="flex items-center gap-2">
          <input
            type="checkbox"
            checked={pub}
            onChange={(e) => setPublic(e.target.checked)}
          />
          Public
        </label>
        <label className="block space-y-2">
          HTML
          <Textarea
            className="font-mono text-sm"
            rows={24}
            value={html}
            onChange={(e) => setHTML(e.target.value)}
          />
        </label>
        {error && <Status error>{error}</Status>}
      </form>
      {!creating && (
        <details className="mt-6">
          <summary>Change with the agent</summary>
          <div className="mt-4">
            <Form
              fields={[
                {
                  name: "instruction",
                  label: "What should change?",
                  type: "textarea",
                  required: true,
                },
              ]}
              label="Update"
              submit={async (v) => {
                const r = await json<any>(
                  `/apps/${encodeURIComponent(app.slug)}/ai-edit`,
                  {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(v),
                  },
                );
                setHTML(r.html);
              }}
            />
          </div>
        </details>
      )}
    </>
  );
}

function Versions() {
  const slug = location.pathname.split("/")[2];
  const { data, error } = useData<any>(() => json(location.pathname));
  return (
    <>
      <PageHeading
        title="App versions"
        actions={
          <Button asChild>
            <a href={"/apps/" + encodeURIComponent(slug) + "/edit"}>Editor</a>
          </Button>
        }
      />
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.slice().reverse()}
        render={(v) => (
          <>
            <h2 className="font-medium">
              Version {v.number} · {v.name}
            </h2>
            <p>{v.summary}</p>
            <p className="text-sm text-muted-foreground">
              <When value={v.saved_at} />
            </p>
            <Action
              run={async () => {
                if (
                  !confirm(
                    "Restore this version? Your current version will stay in history.",
                  )
                )
                  return;
                await mutate(location.pathname, { version: String(v.number) });
                location.assign("/apps/" + encodeURIComponent(slug) + "/edit");
              }}
            >
              Restore
            </Action>
          </>
        )}
      />
    </>
  );
}
