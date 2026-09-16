import { useState } from "react";
import {
  useData,
  json,
  Button,
  Input,
  Status,
  Form,
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
              {catalogue.some((a) => a.id === name) && (
                <Button asChild>
                  <a href={catalogue.find((a) => a.id === service.name)!.path}>
                    App
                  </a>
                </Button>
              )}
            </div>
            <div className="mb-6 space-y-2 text-sm text-muted-foreground">
              <p>Use a Services token for HTTP API and MCP access. Apps receive the SDK automatically.</p>
              <p>MCP endpoint: <code>{location.origin}/mcp</code>. The playground uses your signed-in session and the service tool dispatcher, and shows the complete JSON response.</p>
              <div className="flex flex-wrap gap-4 pb-2">
                <a className="underline underline-offset-4" href="/token">Manage tokens</a>
                <a className="underline underline-offset-4" href="/service/sdk">SDK setup</a>
              </div>
            </div>
            {service.methods?.map((m) => (
              <MethodView
                key={m.Method}
                service={name}
                method={m}
              />
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
          <nav
            aria-label="Services"
            className="grid grid-cols-3 gap-2 sm:grid-cols-4 lg:grid-cols-6"
          >
            {data
              ?.filter((s) =>
                (s.label + " " + s.description)
                  .toLowerCase()
                  .includes(q.toLowerCase()),
              )
              .map((s) => (
                <a
                  key={s.name}
                  href={"/service/" + s.name}
                  title={s.description}
                  className="flex min-w-0 flex-col items-center gap-2 rounded-xl px-2 py-4 text-center hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <img
                    src={"/" + s.icon}
                    alt=""
                    className="size-8 object-contain"
                  />
                  <span className="max-w-full break-words text-sm font-medium">
                    {s.label}
                  </span>
                </a>
              ))}
          </nav>
          {data && !data.some((s) =>
            (s.label + " " + s.description).toLowerCase().includes(q.toLowerCase()),
          ) && <p className="py-5 text-muted-foreground">No services match.</p>}
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
    <section className="space-y-3 border-t py-4 text-sm" id={m.Tool}>
      <h2 className="text-base font-semibold">{m.Method}</h2>
      <p>{m.Doc}</p>
      <code className="block overflow-x-auto rounded bg-muted p-3 text-sm">
        POST {m.Path}
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
      <div className="grid min-w-0 gap-6 lg:grid-cols-2">
      <div className="min-w-0">
        <div className="space-y-3">
          {(
            <>
              <h3 className="font-medium">HTTP API</h3>
              <pre className="overflow-x-auto rounded bg-muted p-3 text-sm">{`curl -X POST '${location.origin}${m.Path}' \\\n  -H 'Authorization: Bearer <services-token>' \\\n  -H 'Content-Type: application/json' \\\n  --data '{}'`}</pre>
            </>
          )}
          {(
            <>
              <h3 className="font-medium">App SDK</h3>
              <pre className="overflow-x-auto rounded bg-muted p-3 text-sm">{`await mu.service(${JSON.stringify(service)}, ${JSON.stringify(m.Method.toLowerCase())}, {});`}</pre>
            </>
          )}
          {(
            <>
              <h3 className="font-medium">MCP</h3>
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
            </>
          )}
          <p className="text-sm text-muted-foreground">
            Replace the empty argument object with the parameters listed above.
          </p>
        </div>
      </div>
      <div className="min-w-0">
        <h3 className="font-medium">Playground</h3>
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
              setResult(response);
            }}
          />
          {result !== undefined && (
            <pre className="mt-4 max-h-96 overflow-auto rounded bg-muted p-3 text-sm">
              {JSON.stringify(result, null, 2)}
            </pre>
          )}
        </div>
      </div>
      </div>
    </section>
  );
}
