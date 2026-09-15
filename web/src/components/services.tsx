import { useState } from "react";
import {
  useData,
  json,
  call,
  Button,
  Input,
  Status,
  Form,
  Read,
  Link,
} from "../../../apps/shared";
import { PageHeading } from "./layout";
import catalogue from "../../../apps/catalog.json";
type Param = {
  name: string;
  type: string;
  description: string;
  required: boolean;
};
type Method = {
  Method: string;
  Doc: string;
  Tool: string;
  Path: string;
  Cost: number;
  NeedsAuth: boolean;
  Changes: boolean;
  PrivateSearch: boolean;
  Params: Param[];
};
type Service = {
  name: string;
  label: string;
  description: string;
  page: string;
  methods: Method[];
};
export function Services() {
  const { data, error } = useData<Service[]>(() => json("/client/services"));
  const [q, setQ] = useState("");
  const name = location.pathname.split("/")[2];
  const service = data?.find((s) => s.name === name);
  return (
    <>
      <PageHeading
        title={service?.label || "Services"}
        actions={
          <Button asChild>
            <a href="/apps">Apps</a>
          </Button>
        }
      />
      {error && <Status error>{error}</Status>}
      {name ? (
        service ? (
          <>
            <p className="mb-5 text-muted-foreground">{service.description}</p>
            <div className="mb-6 flex flex-wrap gap-2">
              <Button asChild>
                <a href="/services">All services</a>
              </Button>
              {catalogue.some((a) => a.id === name) && (
                <Button asChild>
                  <a href={service.page}>Open app</a>
                </Button>
              )}
              <Button asChild>
                <a href="/api">API</a>
              </Button>
              <Button asChild>
                <a href="/apps/sdk.js">App SDK</a>
              </Button>
            </div>
            <p className="mb-6">
              These capabilities are available to your agents and apps. Service
              tokens select the service API; Agent, Work and Inbox use the
              public outcome API.
            </p>
            {service.methods?.map((m) => (
              <MethodView key={m.Method} service={name} method={m} />
            ))}
          </>
        ) : (
          <Status>{data ? "No such service." : "Loading…"}</Status>
        )
      ) : (
        <>
          <Input
            className="mb-6 max-w-xl"
            aria-label="Find a service"
            placeholder="Find a service"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2 xl:grid-cols-3">
            {data
              ?.filter((s) =>
                (s.label + " " + s.description)
                  .toLowerCase()
                  .includes(q.toLowerCase()),
              )
              .map((s) => (
                <section key={s.name} className="space-y-2 border-b pb-5">
                  <h2 className="font-medium">
                    <Link url={"/services/" + s.name}>{s.label}</Link>
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    {s.description}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button asChild>
                      <a href={"/services/" + s.name}>API & SDK</a>
                    </Button>
                    {catalogue.some((a) => a.id === s.name) && (
                      <Button asChild>
                        <a href={s.page}>Open app</a>
                      </Button>
                    )}
                  </div>
                </section>
              ))}
          </div>
        </>
      )}
    </>
  );
}
function MethodView({
  service,
  method: m,
}: {
  service: string;
  method: Method;
}) {
  const [result, setResult] = useState<any>();
  return (
    <section className="space-y-4 border-t py-5" id={m.Tool}>
      <h2 className="text-lg font-medium">{m.Method}</h2>
      <p>{m.Doc}</p>
      <code className="block overflow-x-auto rounded bg-muted p-3 text-sm">
        {m.Changes || m.PrivateSearch ? "POST" : "GET"} {m.Path}
      </code>
      {m.Cost > 0 && (
        <p className="text-sm text-muted-foreground">
          {m.Cost} credits per call
        </p>
      )}
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead>
            <tr>
              <th className="p-2">Parameter</th>
              <th className="p-2">Type</th>
              <th className="p-2">Description</th>
            </tr>
          </thead>
          <tbody>
            {m.Params?.map((p) => (
              <tr key={p.name} className="border-t">
                <td className="p-2">
                  {p.name}
                  {p.required ? " *" : ""}
                </td>
                <td className="p-2">{p.type}</td>
                <td className="p-2">{p.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <details>
        <summary>Try it</summary>
        <div className="mt-4 max-w-xl">
          <Form
            label="Run"
            fields={
              m.Params?.map((p) => ({
                name: p.name,
                label: p.name,
                help: p.description,
                required: p.required,
                type:
                  p.type === "number" || p.type === "integer"
                    ? "number"
                    : "text",
                options:
                  p.type === "boolean" ? ["", "true", "false"] : undefined,
              })) || []
            }
            submit={async (v) => {
              const args: Record<string, unknown> = {};
              for (const p of m.Params || []) {
                if (v[p.name] !== "")
                  args[p.name] =
                    p.type === "number" || p.type === "integer"
                      ? Number(v[p.name])
                      : p.type === "boolean"
                        ? v[p.name] === "true"
                        : v[p.name];
              }
              setResult(await call(service, m.Method.toLowerCase(), args));
            }}
          />
          {result !== undefined && (
            <pre className="mt-4 max-h-96 overflow-auto rounded bg-muted p-3 text-sm">
              {JSON.stringify(result, null, 2)}
            </pre>
          )}
        </div>
      </details>
    </section>
  );
}
