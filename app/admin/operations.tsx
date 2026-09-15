import { useState } from "react";
import {
  Button,
  Input,
  StateBadge,
  When,
  Read,
  Action,
  Form,
  mutate,
} from "../shared";

function Table({
  headings,
  rows,
}: {
  headings: string[];
  rows: React.ReactNode[][];
}) {
  return rows.length ? (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead>
          <tr>
            {headings.map((h) => (
              <th key={h} className="border-b px-3 py-2 font-semibold">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i}>
              {row.map((v, j) => (
                <td key={j} className="border-b px-3 py-3 align-top">
                  {v}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  ) : (
    <p className="py-4 text-muted-foreground">No records.</p>
  );
}
function Metrics({ items }: { items: [string, React.ReactNode][] }) {
  return (
    <dl className="grid grid-cols-2 gap-4 border-b pb-5 sm:grid-cols-4">
      {items.map(([label, value]) => (
        <div key={label}>
          <dt className="text-sm text-muted-foreground">{label}</dt>
          <dd className="mt-1 text-xl font-semibold">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
export function Health({ data }: { data: any }) {
  const checks = data.checks || [];
  const issues = checks.filter((c: any) => c.Status !== "ok").length;
  return (
    <section className="space-y-4">
      <div className="flex flex-wrap gap-2">
        <Button asChild>
          <a href="/admin/status?diagnose=1">Run AI diagnosis</a>
        </Button>
        <Button asChild>
          <a href="/admin/status?test=digest">Test digest</a>
        </Button>
        <Button asChild>
          <a href="/admin/status?test=federation">Test federation</a>
        </Button>
      </div>
      {data.diagnosis && <Read>{data.diagnosis}</Read>}
      <p className="font-medium">
        {issues ? `${issues} ${issues === 1 ? "check needs" : "checks need"} attention` : "All systems operational"}
      </p>
      <div className="divide-y">
        {checks.map((c: any) => (
          <article key={c.Name} className="space-y-2 py-4">
            <div className="flex items-center justify-between gap-3">
              <h2 className="font-semibold">{c.Name}</h2>
              <StateBadge value={c.Status} />
            </div>
            <p className="text-sm">{c.Detail}</p>
            {c.Fix && <p className="text-sm text-muted-foreground">{c.Fix}</p>}
          </article>
        ))}
      </div>
    </section>
  );
}
export function Server({ data }: { data: any }) {
  const s = data.status;
  return (
    <div className="space-y-6">
      <Metrics
        items={[
          ["Uptime", s.uptime],
          ["Memory", `${s.memory.alloc_mb} MB`],
          ["Disk used", `${s.disk.percent.toFixed(1)}%`],
          ["Online", s.online_users],
        ]}
      />
      <section>
        <h2 className="mb-3 font-semibold">Services</h2>
        <Table
          headings={["Service", "Health", "Details"]}
          rows={(s.services || []).map((c: any) => [
            c.name,
            <StateBadge value={c.status ? "healthy" : "error"} />,
            c.details,
          ])}
        />
      </section>
      <section>
        <h2 className="mb-3 font-semibold">Storage</h2>
        <Table
          headings={["Store", "Size", "Files"]}
          rows={(data.stores || []).map((v: any) => [
            v.Name,
            `${(v.Size / 1048576).toFixed(2)} MB`,
            v.Files,
          ])}
        />
      </section>
    </div>
  );
}
export function Backups({ data }: { data: any }) {
  return (
    <div className="space-y-5">
      <Metrics
        items={[
          ["Snapshots", data.snapshots?.length || 0],
          ["Offsite backup", data.offsite ? "Enabled" : "Disabled"],
          [
            "Last upload",
            data.last_push && !String(data.last_push).startsWith("0001") ? (
              <When value={data.last_push} />
            ) : (
              "Never"
            ),
          ],
          ["Quarantined", data.quarantined?.length || 0],
        ]}
      />
      {data.failure && (
        <p role="alert" className="text-destructive">
          {data.failure}
        </p>
      )}
      <Table
        headings={["Snapshot", "Taken", "Files", "Size"]}
        rows={(data.snapshots || []).map((s: any) => [
          s.Name,
          <When value={s.At} />,
          s.Files,
          `${(s.Bytes / 1048576).toFixed(2)} MB`,
        ])}
      />
    </div>
  );
}
export function Logs({ data }: { data: any }) {
  const [query, setQuery] = useState("");
  const tab = new URLSearchParams(location.search).get("tab");
  const matches = (rows: any[]) =>
    rows.filter((r) =>
      JSON.stringify(r).toLowerCase().includes(query.toLowerCase()),
    );
  return (
    <section className="space-y-4">
      <Input
        type="search"
        aria-label="Filter logs"
        placeholder="Filter these logs"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      {tab === "mail" ? (
        <>
          <h2 className="font-semibold">Received mail</h2>
          <Table
            headings={["Time", "From", "To", "Subject"]}
            rows={matches(data.messages || []).map((r) => [
              <When value={r.time} />,
              r.from,
              r.to,
              r.subject,
            ])}
          />
          <h2 className="font-semibold">Delivery attempts</h2>
          <Table
            headings={["Time", "Recipient", "Result"]}
            rows={matches(data.relays || []).map((r) => [
              <When value={r.time || r.Time} />,
              r.to || r.To,
              r.error || (r.ok ? "Delivered" : "Failed"),
            ])}
          />
        </>
      ) : tab === "api" ? (
        <Table
          headings={[
            "Time",
            "Service",
            "Operation",
            "Status",
            "Duration",
            "Detail",
          ]}
          rows={matches(data || []).map((r) => [
            <When value={r.time} />,
            r.service,
            r.method,
            <StateBadge value={String(r.status)} />,
            `${Math.round(r.duration / 1e6)} ms`,
            r.error || r.outcome || "—",
          ])}
        />
      ) : (
        <Table
          headings={["Time", "Source", "Message"]}
          rows={matches(data || []).map((r) => [
            <When value={r.Time} />,
            r.Package,
            <span
              className={r.Alert ? "text-destructive" : "whitespace-pre-wrap"}
            >
              {r.Message}
            </span>,
          ])}
        />
      )}
    </section>
  );
}
export function Traffic({ data }: { data: any }) {
  const series = data.series || [];
  const spend = new URLSearchParams(location.search).get("tab") === "spend";
  if (spend)
    return (
      <section className="space-y-5">
        <Button asChild>
          <a href="/admin/traffic">Requests</a>
        </Button>
        <Metrics
          items={[
            [
              "Provider spend",
              `$${(data.spend.total_cost_cents / 100).toFixed(2)}`,
            ],
            ["Calls", data.spend.total_calls],
            ["Agent runs", data.agent_health.Runs],
            ["Failed runs", data.agent_health.Failed],
          ]}
        />
        <Table
          headings={["Service", "Calls", "Cost (USD)"]}
          rows={(data.spend.by_service || []).map((r: any) => [
            r.service,
            r.calls,
            `$${(r.cost_cents / 100).toFixed(2)}`,
          ])}
        />
        <h2 className="font-semibold">Agent errors</h2>
        <Table
          headings={["Error", "Runs"]}
          rows={(data.agent_health.TopErrors || []).map((r: any) => [
            r.Name,
            r.N,
          ])}
        />
      </section>
    );
  const max = Math.max(1, ...series.map((s: any) => s.total));
  return (
    <div className="space-y-5">
      <Button asChild>
        <a href="/admin/traffic?tab=spend">Spend and agent runs</a>
      </Button>
      <nav className="flex flex-wrap gap-2" aria-label="Traffic period">
        {[
          ["live", "Last 2 hours"],
          ["week", "Last 7 days"],
          ["quarter", "Last 90 days"],
        ].map(([w, label]) => (
          <Button key={w} asChild>
            <a href={`/admin/traffic?window=${w}`}>{label}</a>
          </Button>
        ))}
      </nav>
      <Metrics
        items={[
          [
            "Requests in period",
            series.reduce((n: number, s: any) => n + s.total, 0),
          ],
          ["Peak interval", max],
        ]}
      />
      <figure>
        <figcaption className="mb-2 text-sm font-medium">
          Requests over time
        </figcaption>
        <svg
          viewBox="0 0 800 160"
          role="img"
          aria-label="Request volume by time"
          className="h-40 w-full"
        >
          {series.map((s: any, i: number) => (
            <rect
              key={i}
              x={(i * 800) / series.length}
              y={160 - (s.total / max) * 150}
              width={Math.max(1, 800 / series.length - 2)}
              height={(s.total / max) * 150}
              fill="currentColor"
            >
              <title>
                {new Date(s.at).toLocaleString()}: {s.total} requests
              </title>
            </rect>
          ))}
        </svg>
      </figure>
      {[
        ["Endpoints", data.endpoints],
        ["Callers", data.callers],
        ["Surfaces", data.surfaces],
      ].map(([title, rows]) => (
        <section key={title as string}>
          <h2 className="mb-2 font-semibold">{title}</h2>
          <Table
            headings={[title as string, "Requests"]}
            rows={(rows || []).map((r: any) => [r.key, r.count])}
          />
        </section>
      ))}
    </div>
  );
}
export function Alerts({ data }: { data: any }) {
  return (
    <div className="space-y-5">
      <Metrics
        items={[
          ["Delivery", data.enabled ? "Enabled" : "Disabled"],
          ["Alerts", data.alerts],
          ["Calls in last hour", data.calls_last_hour],
        ]}
      />
      <h2 className="font-semibold">Recipients</h2>
      <p className="text-sm">
        {(data.recipients || []).join(", ") || "No recipients configured"}
      </p>
      <p className="text-sm text-muted-foreground">
        Test alerts use the normal delivery path and cooldown.
      </p>
    </div>
  );
}
export function Spam({
  data,
  refresh,
}: {
  data: any;
  refresh: () => Promise<void>;
}) {
  const f = data.filter;
  const update = async (action: string, value = "") => {
    await mutate("/admin/spam", { action, value, email: value, ip: value });
    await refresh();
  };
  const lists: {
    title: string;
    items: string[];
    add: string;
    remove: string;
    label: string;
  }[] = [
    {
      title: "Blocked senders",
      items: data.blocklist?.emails || [],
      add: "block_email",
      remove: "unblock_email",
      label: "Address or domain",
    },
    {
      title: "Blocked IPs",
      items: data.blocklist?.ips || [],
      add: "block_ip",
      remove: "unblock_ip",
      label: "IP address",
    },
    {
      title: "Blocked domains",
      items: f.blocked_tlds || [],
      add: "add_tld",
      remove: "remove_tld",
      label: "Top-level domain",
    },
    {
      title: "Blocked keywords",
      items: f.blocked_keywords || [],
      add: "add_keyword",
      remove: "remove_keyword",
      label: "Keyword",
    },
    {
      title: "Allowed senders",
      items: f.allowed_senders || [],
      add: "add_allowed",
      remove: "remove_allowed",
      label: "Address or domain",
    },
  ];
  return (
    <div className="space-y-6">
      <section className="space-y-4">
        <h2 className="font-semibold">Filtering</h2>
        <div className="flex flex-wrap gap-2">
          <Action run={() => update("toggle")}>
            {f.enabled ? "Disable filter" : "Enable filter"}
          </Action>
          <Action run={() => update("toggle_reject")}>
            {f.reject_spam
              ? "Switch to silent drop"
              : "Save to filtered folder"}
          </Action>
          <Action run={() => update("toggle_autoblock")}>
            {f.auto_block_domains
              ? "Disable automatic domain blocking"
              : "Enable automatic domain blocking"}
          </Action>
        </div>
        <Form
          label="Set threshold"
          fields={[
            {
              name: "value",
              label: "Spam score threshold",
              type: "number",
              value: String(f.threshold),
              required: true,
            },
          ]}
          submit={(v) => update("set_threshold", v.value)}
        />
      </section>
      {lists.map((list) => (
        <section key={list.title} className="space-y-3 border-t pt-5">
          <h2 className="font-semibold">{list.title}</h2>
          {list.items.length ? (
            <ul className="divide-y">
              {list.items.map((value) => (
                <li
                  key={value}
                  className="flex items-center justify-between gap-3 py-2"
                >
                  <span className="min-w-0 break-all text-sm">{value}</span>
                  <Action run={() => update(list.remove, value)}>Remove</Action>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">None</p>
          )}
          <details>
            <summary className="cursor-pointer text-sm font-medium">
              Add
            </summary>
            <div className="mt-3">
              <Form
                label="Add"
                fields={[{ name: "value", label: list.label, required: true }]}
                submit={(v) => update(list.add, v.value)}
              />
            </div>
          </details>
        </section>
      ))}
    </div>
  );
}
