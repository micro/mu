import {
  Health,
  Server,
  Backups,
  Logs,
  Traffic,
  Alerts,
  Spam,
} from "./operations";
import { useState } from "react";
import {
  useData,
  json,
  mutate,
  Button,
  Input,
  Status,
  Form,
  Rows,
  Read,
  Action,
  When,
  StateBadge,
} from "../shared";
import { PageHeading } from "../../web/src/components/layout";
const pages = [
  "alerts",
  "backup",
  "config",
  "log",
  "moderate",
  "oauth",
  "server",
  "spam",
  "status",
  "traffic",
  "users",
];
export function Admin() {
  const name = location.pathname.split("/")[2] || "";
  const [social, setSocial] = useState(false);
  const { data, error, refresh } = useData<any>(
    () =>
      name
        ? json(
            name === "log"
              ? "/admin/client?page=log&" + location.search.slice(1)
              : `/admin/client?${new URLSearchParams({ ...Object.fromEntries(new URLSearchParams(location.search)), page: name, ...(social ? { source: "social" } : {}) })}`,
          )
        : Promise.resolve(null),
    [name, social],
  );
  return (
    <>
      <PageHeading
        title={name ? name[0].toUpperCase() + name.slice(1) : "Admin"}
        actions={
          name ? (
            <Button asChild>
              <a href="/admin">Admin</a>
            </Button>
          ) : undefined
        }
      />
      {!name ? (
        <nav className="grid gap-4 sm:grid-cols-2">
          {pages.map((p) => (
            <a className="border-b pb-3" key={p} href={"/admin/" + p}>
              {p[0].toUpperCase() + p.slice(1)}
            </a>
          ))}
        </nav>
      ) : name === "config" ? (
        data && <Config groups={data.groups} />
      ) : (
        <>
          {name === "log" && (
            <div className="flex flex-wrap gap-2">
              {[
                ["", "System"],
                ["api", "External calls"],
                ["mail", "Mail"],
              ].map(([tab, label]) => (
                <Button key={tab} asChild>
                  <a href={"/admin/log?tab=" + tab}>{label}</a>
                </Button>
              ))}
            </div>
          )}
          {name === "backup" && (
            <Action
              run={async () => {
                await mutate("/admin/backup", {});
                await refresh();
              }}
            >
              Back up now
            </Action>
          )}
          {name === "alerts" && (
            <Action run={() => mutate("/admin/alerts", {})}>
              Send test alert
            </Action>
          )}
          {name === "moderate" && (
            <label className="mb-5 flex items-center gap-2">
              <input
                type="checkbox"
                checked={social}
                onChange={(e) => setSocial(e.target.checked)}
              />
              Include imported social content
            </label>
          )}
          {name === "moderate" ? (
            <Rows
              items={data?.items}
              render={(f) => (
                <>
                  <h2 className="font-medium">
                    {f.content_type} · {f.content_id}
                  </h2>
                  {f.content && (
                    <>
                      <h3>{f.content.title}</h3>
                      <p>{f.content.author}</p>
                      <Read>{f.content.body}</Read>
                    </>
                  )}
                  <p>
                    {f.flag_count} flags · <When value={f.flagged_at} />
                  </p>
                  <div className="flex gap-2">
                    <Action
                      run={async () => {
                        await mutate("/admin/moderate", {
                          action: "approve",
                          type: f.content_type,
                          id: f.content_id,
                        });
                        await refresh();
                      }}
                    >
                      Approve
                    </Action>
                    <Action
                      danger
                      run={async () => {
                        await mutate("/admin/moderate", {
                          action: "delete",
                          type: f.content_type,
                          id: f.content_id,
                        });
                        await refresh();
                      }}
                    >
                      Delete
                    </Action>
                  </div>
                </>
              )}
            />
          ) : name === "oauth" ? (
            <Rows
              items={data?.clients}
              render={(c) => (
                <>
                  <h2 className="font-medium">{c.name}</h2>
                  <p className="break-all text-sm text-muted-foreground">
                    {c.id} · {c.owner}
                  </p>
                  <Form
                    fields={[
                      {
                        name: "redirect_uri",
                        label: "Redirect URI",
                        value: c.redirects?.[0] || "",
                        required: true,
                      },
                    ]}
                    submit={async (v) => {
                      await mutate("/admin/oauth", {
                        action: "redirect",
                        client_id: c.id,
                        ...v,
                      });
                      await refresh();
                    }}
                  />
                  <Action
                    danger
                    run={async () => {
                      await mutate("/admin/oauth", { client_id: c.id });
                      await refresh();
                    }}
                  >
                    Remove client
                  </Action>
                </>
              )}
            />
          ) : (
            <div className="mt-5">
              {!data ? (
                <Status>Loading…</Status>
              ) : name === "status" ? (
                <Health data={data} />
              ) : name === "server" ? (
                <Server data={data} />
              ) : name === "backup" ? (
                <Backups data={data} />
              ) : name === "log" ? (
                <Logs data={data} />
              ) : name === "traffic" ? (
                <Traffic data={data} />
              ) : name === "alerts" ? (
                <Alerts data={data} />
              ) : name === "spam" ? (
                <Spam data={data} refresh={refresh} />
              ) : (
                <Status error>This admin page is unavailable.</Status>
              )}
            </div>
          )}
        </>
      )}
      {error && <Status error>{error}</Status>}
    </>
  );
}
function Config({ groups }: { groups: any[] }) {
  const [busy, setBusy] = useState(false),
    [status, setStatus] = useState("");
  return (
    <form
      className="space-y-6"
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        setStatus("");
        try {
          const values = Object.fromEntries(
            new FormData(e.currentTarget),
          ) as Record<string, string>;
          await mutate("/admin/config", values);
          setStatus("Saved. Settings read at startup need a restart.");
        } catch (e) {
          setStatus((e as Error).message);
        } finally {
          setBusy(false);
        }
      }}
    >
      <Button disabled={busy}>{busy ? "Saving…" : "Save"}</Button>
      {status && <Status>{status}</Status>}
      {groups.map((g) => (
        <section key={g.name} className="border-t pt-4">
          <h2 className="font-medium">{g.name}</h2>
          <p className="my-3 text-sm text-muted-foreground">{g.description}</p>
          <div className="grid gap-4 sm:grid-cols-2">
            {g.fields.map((f: any) => (
              <label key={f.name} className="block min-w-0 space-y-2">
                <span className="break-all text-sm">
                  {f.name} · {f.source}
                </span>
                <Input
                  name={f.name}
                  type={f.secret ? "password" : "text"}
                  disabled={f.source === "env"}
                  defaultValue={f.value}
                  placeholder={
                    f.secret && f.set ? "Set — leave blank to keep" : ""
                  }
                  autoComplete="off"
                />
                {f.secret && f.set && f.source !== "env" && (
                  <span className="flex items-center gap-2 text-sm">
                    <input type="checkbox" name={"clear_" + f.name} value="1" />
                    Clear stored value
                  </span>
                )}
              </label>
            ))}
          </div>
        </section>
      ))}
      <Button disabled={busy}>Save</Button>
    </form>
  );
}
