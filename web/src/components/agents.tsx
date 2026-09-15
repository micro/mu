import { useState } from "react";
import {
  useData,
  json,
  Button,
  Input,
  Textarea,
  NativeSelect,
  Status,
  Rows,
  Action,
  Link,
  mutate,
} from "../../../app/shared";
import { PageHeading } from "./layout";
export function Agents() {
  const { data, error, refresh } = useData<any>(() => json("/client/agents"));
  const { data: services } = useData<any[]>(() => json("/client/services"), [], "services");
  const params = new URLSearchParams(location.search),
    editing = location.pathname === "/agent/new",
    id = params.get("id") || params.get("fork"),
    selected = data?.agents?.find((a: any) => a.id === id);
  return (
    <>
      <PageHeading
        title={editing ? (params.has("id") ? "Edit agent" : "New agent") : "Agents"}
        actions={
          !editing ? (
            <Button asChild>
              <a href="/agent/new">New</a>
            </Button>
          ) : undefined
        }
      />
      {error && <Status error>{error}</Status>}
      {editing ? (
        data && (
          <AgentEditor
            agent={selected}
            fork={params.has("fork")}
            services={services || []}
            models={data.models || []}
          />
        )
      ) : (
        <>
          <Rows
            items={data?.agents}
            render={(a) => (
              <>
                <h2 className="font-medium">
                  <Link url={"/agent/" + encodeURIComponent(a.id)}>
                    {a.name}
                  </Link>
                </h2>
                <p className="text-muted-foreground">{a.description}</p>
                <div className="flex flex-wrap gap-2">
                  <Button asChild>
                    <a href={"/agent/" + encodeURIComponent(a.id)}>Chat</a>
                  </Button>
                  <Button asChild>
                    <a href={"/agent/new?id=" + encodeURIComponent(a.id)}>
                      Edit
                    </a>
                  </Button>
                  <Button asChild>
                    <a href={"/agent/connect?id=" + encodeURIComponent(a.id)}>
                      Connect
                    </a>
                  </Button>
                  <Action
                    danger
                    run={async () => {
                      await mutate("/agents/data", {
                        action: "delete",
                        id: a.id,
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
          <h2 className="my-5 text-lg font-medium">Built in</h2>
          <Rows
            items={data?.builtins?.map((a: any) => ({
              id: a.ID,
              name: a.Name,
              description: a.Description,
            }))}
            render={(a) => (
              <>
                <h3 className="font-medium">
                  <Link url={"/agent/" + encodeURIComponent(a.id)}>
                    {a.name}
                  </Link>
                </h3>
                <p className="text-muted-foreground">{a.description}</p>
              </>
            )}
          />
        </>
      )}
    </>
  );
}
function AgentEditor({
  agent,
  fork,
  services,
  models,
}: {
  agent: any;
  fork: boolean;
  services: any[];
  models: any[];
}) {
  const [mode, setMode] = useState(
      !agent || agent.tools?.length ? "select" : "all",
    ),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <form
      className="max-w-3xl space-y-6"
      onSubmit={async (e) => {
        e.preventDefault();
        const values = new URLSearchParams(
          new FormData(e.currentTarget) as any,
        );
        values.set("action", "save");
        values.set("scope_mode", mode);
        if (agent && !fork) values.set("id", agent.id);
        setBusy(true);
        try {
          const saved = await json<any>("/agents/data", {
            method: "POST",
            body: values,
          });
          location.assign("/agent/" + encodeURIComponent(saved.id));
        } catch (e) {
          setError((e as Error).message);
        } finally {
          setBusy(false);
        }
      }}
    >
      <div className="grid gap-6 sm:grid-cols-2">
      <label className="block min-w-0 space-y-2">
        <span className="block text-sm font-medium">Name</span>
        <Input
          name="name"
          required
          defaultValue={(fork ? "Copy of " : "") + (agent?.name || "")}
        />
      </label>
      <label className="block space-y-2">
        <span className="block text-sm font-medium">Description</span>
        <Input name="description" defaultValue={agent?.description} />
      </label>
      </div>
      <label className="block space-y-2">
        <span className="block text-sm font-medium">Instructions</span>
        <Textarea
          name="prompt"
          required
          rows={8}
          defaultValue={agent?.prompt}
        />
      </label>
      <section className="space-y-5 border-t pt-5">
      <h2 className="font-medium">Model and tools</h2>
      <div className="grid gap-5 sm:grid-cols-2">
      <label className="block min-w-0 space-y-2">
        <span className="block text-sm font-medium">Model</span>
        <NativeSelect name="model" defaultValue={agent?.model || ""}>
          <option value="">Default</option>
          {models.map((m) => (
            <option key={m.id} value={m.id}>
              {m.label}
            </option>
          ))}
        </NativeSelect>
      </label>
      <label className="block space-y-2">
        <span className="block text-sm font-medium">Service access</span>
        <NativeSelect value={mode} onChange={(e) => setMode(e.target.value)}>
          <option value="all">All services</option>
          <option value="select">Select services</option>
        </NativeSelect>
      </label>
      </div>
      {mode === "select" && (
        <fieldset className="grid gap-x-6 gap-y-3 sm:grid-cols-2">
          <legend className="mb-3 text-sm">Services this agent can use</legend>
          {services.map((s) => (
            <label key={s.name} className="flex min-w-0 items-start gap-3 text-sm">
              <input
                className="mt-1 size-4 shrink-0"
                type="checkbox"
                name="tools"
                value={s.name}
                defaultChecked={agent?.tools?.includes(s.name)}
              />
              {s.label}
            </label>
          ))}
        </fieldset>
      )}
      </section>
      <div className="flex gap-2 border-t pt-5">
        <Button disabled={busy}>{busy ? "Saving…" : "Save"}</Button>
        <Button asChild>
          <a href="/agents">Cancel</a>
        </Button>
      </div>
      {error && <Status error>{error}</Status>}
    </form>
  );
}
