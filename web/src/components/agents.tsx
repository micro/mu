import { useState } from "react";
import {
  useData,
  json,
  Button,
  Input,
  Textarea,
  NativeSelect,
  Status,
  Action,
  Link,
  mutate,
} from "../../../app/shared";
import { Card } from "./ui/card";
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
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {data?.agents?.map((a: any) => (
              <Card key={a.id}>
                <h2 className="flex items-center gap-3 text-lg font-medium">
                  <img src="/agent.svg" alt="" className="size-8 shrink-0" />
                  <Link url={"/agent/" + encodeURIComponent(a.id)}>
                    {a.name}
                  </Link>
                </h2>
                <p className="break-words text-sm text-muted-foreground">{a.description}</p>
                <div className="mt-auto flex flex-wrap gap-2 pt-2">
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
              </Card>
            ))}
          </div>
          {data?.agents?.length === 0 && <p className="py-4 text-muted-foreground">No agents yet. Create one, or try a built-in agent below.</p>}
          <h2 className="mb-4 mt-8 text-lg font-medium">Built-in agents</h2>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {data?.builtins?.map((a: any) => (
              <Card key={a.ID}>
                <h3 className="flex items-center gap-3 text-lg font-medium">
                  <img src="/agent.svg" alt="" className="size-8 shrink-0" />
                  <Link url={"/agent/" + encodeURIComponent(a.ID)}>{a.Name}</Link>
                </h3>
                <p className="break-words text-sm text-muted-foreground">{a.Description}</p>
                <div className="mt-auto flex flex-wrap gap-2 pt-2">
                  <Button asChild><a href={"/agent/" + encodeURIComponent(a.ID)}>Chat</a></Button>
                  <Button asChild><a href={"/agent/connect?id=" + encodeURIComponent(a.ID)}>Connect</a></Button>
                </div>
              </Card>
            ))}
          </div>
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
