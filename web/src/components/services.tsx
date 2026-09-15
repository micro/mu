import { useState } from "react";
import {
  useData,
  json,
  Button,
  Input,
  Status,
  Form,
  Link,
} from "../../../app/shared";
import { PageHeading } from "./layout";
import catalogue from "../../../app/catalog.json";
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
  Destructive: boolean;
  PrivateSearch: boolean;
  Params: Param[];
};
type Service = {
  name: string;
  label: string;
  description: string;
  page: string;
  icon: string;
  methods: Method[];
};
import { SDKGuide } from "./sdk-guide";
export function Services() {
  if (["/service/sdk", "/services/sdk"].includes(location.pathname))
    return <SDKGuide />;
  return <ServiceReference />;
}
function ServiceReference() {
  const { data, error } = useData<Service[]>(() => json("/client/services"));
  const [q, setQ] = useState("");
  const name = location.pathname.split("/")[2];
  const service = data?.find((s) => s.name === name);
  return (
    <>
      <PageHeading title={service?.label || "Services"} />
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
                  <a href={catalogue.find((a) => a.id === service.name)!.path}>
                    Open app
                  </a>
                </Button>
              )}
              <Button asChild>
                <a href="/mcp">MCP connection</a>
              </Button>
              <Button asChild>
                <a href="/api">API</a>
              </Button>
              <Button asChild>
                <a href="/service/sdk">App SDK</a>
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
                  <h2 className="flex items-center gap-2 font-medium">
                    <img
                      src={"/" + s.icon}
                      alt=""
                      className="size-5"
                      aria-hidden="true"
                    />
                    <Link url={"/service/" + s.name}>{s.label}</Link>
                  </h2>
                  <p className="text-sm text-muted-foreground">
                    {s.description}
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Button asChild>
                      <a href={"/service/" + s.name}>API & SDK</a>
                    </Button>
                    {catalogue.some((a) => a.id === s.name) && (
                      <Button asChild>
                        <a href={catalogue.find((a) => a.id === s.name)!.path}>
                          Open app
                        </a>
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
        <summary className="cursor-pointer">API, SDK and MCP examples</summary>
        <div className="mt-3 space-y-4">
          <p className="text-sm">
            For service access, create a Services token with access to this
            service. Keep credentials on your server, never in shared app
            source.
          </p>
          <a className="underline" href="/token">
            Manage tokens
          </a>
          <h3 className="font-medium">HTTP API</h3>
          <pre className="overflow-x-auto rounded bg-muted p-3 text-sm">{`curl -X POST '${location.origin}${m.Path}' \\\n  -H 'Authorization: Bearer <services-token>' \\\n  -H 'Content-Type: application/json' \\\n  --data '{}'`}</pre>
          <h3 className="font-medium">App SDK</h3>
          <pre className="overflow-x-auto rounded bg-muted p-3 text-sm">{`await mu.service(${JSON.stringify(service)}, ${JSON.stringify(m.Method.toLowerCase())}, {});`}</pre>
          <h3 className="font-medium">MCP</h3>
          <p className="text-sm">
            Connect to {location.origin}/mcp using the same Services token. Use
            tools/list to discover your permitted tools.
          </p>
          <pre className="overflow-x-auto rounded bg-muted p-3 text-sm">
            {JSON.stringify(
              {
                jsonrpc: "2.0",
                id: 1,
                method: "tools/call",
                params: { name: m.Tool, arguments: {} },
              },
              null,
              2,
            )}
          </pre>
          <p className="text-sm text-muted-foreground">
            Replace the empty argument object with the parameters listed above.
          </p>
        </div>
      </details>
      <details>
        <summary className="cursor-pointer">Playground</summary>
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
                        : p.type === "object" || p.type === "array"
                          ? JSON.parse(v[p.name])
                          : v[p.name];
              }
              if (
                m.Destructive &&
                !confirm(
                  "Run " + m.Method + "? This changes or deletes stored data.",
                )
              )
                return;
              const response = await json<any>(
                `/services/call/${service}/${m.Method.toLowerCase()}`,
                {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify(args),
                },
              );
              setResult(response.data ?? response.result ?? response);
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
