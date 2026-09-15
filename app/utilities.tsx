import { useState } from "react";
import {
  useData,
  json,
  call,
  Button,
  Status,
  Form,
  Rows,
  Read,
  Link,
  When,
  type FieldSpec,
} from "./shared";
import { PageHeading } from "../web/src/components/layout";
import { SSHAccess } from "./files";
type Operation = {
  label: string;
  method: string;
  fields: FieldSpec[];
  service?: string;
};
const field = (
  name: string,
  label: string,
  type = "text",
  required = true,
): FieldSpec => ({ name, label, type, required });
const operations: Record<string, Operation[]> = {
  text: [
    {
      label: "Summarise",
      method: "summarise",
      fields: [
        field("text", "Text", "textarea"),
        { name: "style", label: "Style", options: ["prose", "bullets"] },
      ],
    },
    {
      label: "Translate",
      method: "translate",
      fields: [field("text", "Text", "textarea"), field("to", "Language")],
    },
    {
      label: "Extract",
      method: "extract",
      fields: [
        field("text", "Text", "textarea"),
        field("schema", "Fields to extract", "textarea"),
      ],
    },
    {
      label: "Classify",
      method: "classify",
      fields: [
        field("text", "Text", "textarea"),
        field("labels", "Categories, separated by commas"),
      ],
    },
  ],
  shell: [
    {
      label: "Run",
      method: "run",
      fields: [
        field("command", "Command", "textarea"),
        { name: "dir", label: "Directory", value: "/work" },
        {
          name: "timeout",
          label: "Time limit in seconds",
          type: "number",
          value: "120",
        },
      ],
    },
    {
      label: "Write file",
      method: "write",
      fields: [field("path", "Path"), field("content", "Contents", "textarea")],
    },
  ],
  browser: [
    {
      label: "Read page",
      method: "read",
      fields: [
        field("url", "Page URL", "url"),
        field("wait", "Wait for selector", "text", false),
      ],
    },
    {
      label: "Screenshot",
      method: "shot",
      fields: [field("url", "Page URL", "url")],
    },
  ],
  food: [
    {
      label: "Search products",
      method: "search",
      fields: [field("query", "Food or brand")],
    },
    {
      label: "Barcode",
      method: "product",
      fields: [field("barcode", "Barcode")],
    },
  ],
  images: [
    {
      label: "Find images",
      method: "search",
      fields: [field("query", "Describe the image")],
    },
    {
      label: "Generate",
      method: "generate",
      fields: [field("prompt", "Describe your image", "textarea")],
    },
  ],
  weather: [
    {
      label: "Forecast",
      method: "forecast",
      fields: [field("location", "Town or city")],
    },
  ],
  places: [
    {
      label: "Search",
      method: "search",
      fields: [
        field("query", "Place or category"),
        field("near", "Near", "text", false),
      ],
    },
  ],
  flights: [
    {
      label: "Track flight",
      method: "track",
      fields: [field("flight", "Flight number or callsign")],
    },
    {
      label: "Overhead",
      method: "overhead",
      fields: [field("near", "Town or city")],
    },
    {
      label: "Airport",
      method: "airport",
      fields: [field("code", "Airport code")],
    },
  ],
  routes: [
    {
      label: "Directions",
      method: "directions",
      fields: [
        field("from", "From"),
        field("to", "To"),
        {
          name: "mode",
          label: "Travel by",
          options: ["drive", "walk", "cycle", "transit"],
        },
      ],
    },
    {
      label: "Travel time",
      method: "eta",
      fields: [field("from", "From"), field("to", "To")],
    },
  ],
  transit: [
    {
      label: "Find a stop",
      method: "search",
      fields: [field("query", "Stop or station")],
    },
    {
      label: "Trains",
      method: "trains",
      fields: [field("station", "Station code")],
    },
    {
      label: "Arrivals",
      method: "arrivals",
      fields: [field("stop", "Stop name or ID")],
    },
  ],
  prayer: [
    {
      label: "Prayer times",
      method: "times",
      fields: [field("location", "Town or city")],
    },
    {
      label: "Qibla",
      method: "qibla",
      fields: [field("location", "Town or city")],
    },
    {
      label: "Search",
      method: "search",
      fields: [field("query", "Search the Qur’an and hadith")],
    },
  ],
  markets: [
    {
      label: "Prices",
      method: "list",
      fields: [
        {
          name: "category",
          label: "Category",
          options: ["crypto", "stocks", "commodities", "futures", "currencies"],
        },
      ],
    },
    {
      label: "Convert",
      method: "convert",
      fields: [
        { name: "amount", label: "Amount", type: "number", value: "1" },
        field("from", "From currency"),
        field("to", "To currency"),
        field("date", "Date", "date", false),
      ],
    },
  ],
};
export function Utilities({ name }: { name: string }) {
  const ops = operations[name] || [],
    [tab, setTab] = useState(0),
    [result, setResult] = useState<any>();
  const op = ops[tab];
  const { data, error } = useData<any>(
    () =>
      ["hazards", "prayer", "markets"].includes(name)
        ? json("/" + name)
        : Promise.resolve(null),
    [name],
  );
  async function run(v: Record<string, string>) {
    let args: Record<string, unknown> = { ...v };
    for (const f of op.fields)
      if (f.type === "number") args[f.name] = Number(v[f.name]);
    if (v.location) {
      const place = await json<any>(
        "/places?q=" + encodeURIComponent(v.location),
      );
      const p = place.results?.[0] || place.places?.[0] || place;
      args = {
        ...args,
        lat: p.lat ?? p.latitude,
        lon: p.lon ?? p.longitude,
        tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
      };
      if (args.lat === undefined) throw Error("Could not find that location.");
    }
    if (name === "transit" && op.method === "search") {
      setResult(await json("/transit?q=" + encodeURIComponent(v.query)));
      return;
    }
    setResult(await call(op.service || name, op.method, args));
  }
  return (
    <>
      <PageHeading title={name[0].toUpperCase() + name.slice(1)} />
      {name === "shell" && <SSHAccess />}
      {ops.length > 1 && (
        <div className="mb-5 flex flex-wrap gap-2">
          {ops.map((o, i) => (
            <Button
              key={o.method}
              aria-pressed={tab === i}
              variant={i === tab ? "secondary" : "outline"}
              onClick={() => {
                setTab(i);
                setResult(undefined);
              }}
            >
              {o.label}
            </Button>
          ))}
        </div>
      )}
      {op && (
        <div className="mb-6 max-w-3xl">
          <Form
            key={name + op.method}
            fields={op.fields}
            label={op.label}
            submit={run}
          />
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <Output data={result ?? data} />
    </>
  );
}
export function Output({ data }: { data: any }) {
  if (!data) return null;
  const text =
    typeof data === "string"
      ? data
      : data.text ||
        data.output ||
        data.result ||
        data.summary ||
        data.reminder ||
        data.times ||
        data.direction;
  const images =
    data.images ||
    (data.url &&
    /\.(png|jpg|webp)(\?|$)|\/images\/|\/browser\/shot\//.test(data.url)
      ? [data]
      : []);
  const collections = [
    "data",
    "items",
    "results",
    "products",
    "aircraft",
    "stops",
    "arrivals",
    "quakes",
    "alerts",
    "floods",
    "files",
    "users",
  ];
  return (
    <div className="space-y-5">
      {typeof text === "string" && <Read>{text}</Read>}
      {data.path && (
        <p>
          Saved {data.path} · {data.bytes} bytes
        </p>
      )}
      {data.freshness && (
        <p className="text-sm text-muted-foreground">{data.freshness}</p>
      )}
      {data.code !== undefined && (
        <p className="text-sm text-muted-foreground">
          Exit status: {data.code}
        </p>
      )}
      {["verse", "saying", "hadith", "name", "message"].map(
        (k) =>
          typeof data[k] === "string" && (
            <section key={k}>
              <Read>{data[k]}</Read>
            </section>
          ),
      )}
      {images.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2">
          {images.map((im: any, i: number) => (
            <figure key={im.url || i}>
              <a href={im.url}>
                <img
                  className="w-full rounded"
                  src={im.url}
                  alt={im.prompt || im.title || "Image"}
                  loading="lazy"
                />
              </a>
              <figcaption className="mt-2 text-sm text-muted-foreground">
                {im.title || im.prompt}
              </figcaption>
            </figure>
          ))}
        </div>
      )}
      {!text &&
        collections.map(
          (k) =>
            Array.isArray(data[k]) && (
              <section key={k} className="space-y-3">
                <h2 className="text-lg font-medium">
                  {k[0].toUpperCase() + k.slice(1)}
                </h2>
                <Rows
                  items={data[k]}
                  render={(x) => (
                    <>
                      <h3 className="font-medium">
                        <Link url={x.url}>
                          {x.title ||
                            x.name ||
                            x.symbol ||
                            x.callsign ||
                            x.place ||
                            x.description ||
                            x.line ||
                            x.id}
                        </Link>
                      </h3>
                      {x.price !== undefined && (
                        <p>
                          {x.price}{" "}
                          {x.change_24h !== undefined
                            ? `(${x.change_24h}%)`
                            : ""}
                        </p>
                      )}
                      {x.value !== undefined && (
                        <p>
                          {x.value} {x.currency}
                        </p>
                      )}
                      {x.magnitude !== undefined && (
                        <p>Magnitude {x.magnitude}</p>
                      )}
                      {x.altitude !== undefined && (
                        <p>Altitude {x.altitude} ft</p>
                      )}
                      {x.destination && (
                        <p>
                          {x.destination} · {x.expected || x.minutes}
                        </p>
                      )}
                      <Read>
                        {x.text || x.summary || x.note || x.body || ""}
                      </Read>
                      <p className="text-sm text-muted-foreground">
                        <When value={x.at || x.time || x.created} />
                      </p>
                    </>
                  )}
                />
              </section>
            ),
        )}
    </div>
  );
}
