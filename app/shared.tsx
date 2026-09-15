import { useEffect, useRef, useState, type ReactNode, type FormEvent } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { json, mutate, safeURL, initialData } from "../web/src/lib/api";
import { Button } from "../web/src/components/ui/button";
import { Input } from "../web/src/components/ui/input";
import { Textarea } from "../web/src/components/ui/textarea";
import { NativeSelect } from "../web/src/components/ui/select";
import { Status } from "../web/src/components/layout";
export { Button, Input, Textarea, NativeSelect, Status, json, mutate };
export function useData<T>(load: () => Promise<T>, deps: unknown[] = [], initialKey = "page") {
  const [data, setData] = useState<T | undefined>(() => initialData<T>(initialKey)),
    first = useRef(true),
    [error, setError] = useState("");
  const refresh = async () => {
    try {
      setError("");
      setData(await load());
    } catch (e) {
      setError((e as Error).message);
    }
  };
  useEffect(() => {
    if (first.current) { first.current = false; if (initialData(initialKey) !== undefined) return; }
    let live = true;
    setData(undefined);
    setError("");
    load()
      .then((d) => {
        if (live) setData(d);
      })
      .catch((e) => {
        if (live) setError(e.message);
      });
    return () => {
      live = false;
    };
  }, deps);
  return { data, error, setError, refresh, setData };
}
export async function call<T = any>(
  service: string,
  method: string,
  args: Record<string, unknown> = {},
): Promise<T> {
  const response = await json<{ result: T | string; data?: T }>(
    `/client/call/${service}/${method}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(args),
    },
  );
  if (response.data !== undefined) return response.data;
  if (typeof response.result === "string") {
    try {
      return JSON.parse(response.result);
    } catch {
      return { text: response.result } as T;
    }
  }
  return response.result;
}
export function Read({ children }: { children: string }) {
  return (
    <div className="markdown min-w-0 break-words">
      <Markdown
        remarkPlugins={[remarkGfm]}
        components={{ img: ({ alt }) => <span>{alt}</span> }}
      >
        {children}
      </Markdown>
    </div>
  );
}
export function When({ value }: { value?: string }) {
  if (!value || value.startsWith("0001")) return null;
  return (
    <time dateTime={value}>
      {new Date(value).toLocaleString(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      })}
    </time>
  );
}
export function Action({
  children,
  run,
  danger = false,
}: {
  children: ReactNode;
  run: () => Promise<unknown>;
  danger?: boolean;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <>
      <Button
        disabled={busy}
        variant="outline"
        onClick={async () => {
          if (danger && !confirm("Delete this item?")) return;
          setBusy(true);
          setError("");
          try {
            await run();
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? "Working…" : children}
      </Button>
      {error && <Status error>{error}</Status>}
    </>
  );
}
export type FieldSpec = {
  name: string;
  label: string;
  type?: string;
  required?: boolean;
  options?: string[];
  value?: string;
  help?: string;
};
export function Form({
  fields,
  submit,
  label = "Save",
  children,
  initial = {},
}: {
  fields: FieldSpec[];
  submit: (values: Record<string, string>) => Promise<unknown>;
  label?: string;
  children?: ReactNode;
  initial?: Record<string, string>;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const values = Object.fromEntries(new FormData(e.currentTarget)) as Record<
      string,
      string
    >;
    setBusy(true);
    setError("");
    try {
      await submit(values);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form className="space-y-4" onSubmit={save}>
      <fieldset disabled={busy} className="min-w-0 space-y-4">
        {fields.map((f) => (
          <label key={f.name} className="block space-y-2">
            <span className="text-sm font-medium">{f.label}</span>
            {f.type === "textarea" ? (
              <Textarea
                name={f.name}
                required={f.required}
                defaultValue={initial[f.name] ?? f.value}
                rows={8}
              />
            ) : f.options ? (
              <NativeSelect
                name={f.name}
                defaultValue={initial[f.name] ?? f.value}
              >
                {f.options.map((o) => (
                  <option key={o}>{o}</option>
                ))}
              </NativeSelect>
            ) : (
              <Input
                name={f.name}
                type={f.type || "text"}
                required={f.required}
                defaultValue={initial[f.name] ?? f.value}
              />
            )}
            {f.help && (
              <span className="block text-sm text-muted-foreground">
                {f.help}
              </span>
            )}
          </label>
        ))}
        {children}
        <Button type="submit">{busy ? "Working…" : label}</Button>
      </fieldset>
      {error && <Status error>{error}</Status>}
    </form>
  );
}
export function Search({
  onSearch,
  placeholder = "Search",
  initial = "",
}: {
  onSearch: (q: string) => void;
  placeholder?: string;
  initial?: string;
}) {
  const [q, setQ] = useState(initial);
  return (
    <form
      className="mb-5 flex flex-wrap gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        onSearch(q);
      }}
    >
      <Input
        className="min-w-0 flex-[1_1_14rem]"
        aria-label={placeholder}
        placeholder={placeholder}
        type="search"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      <Button type="submit">Search</Button>
    </form>
  );
}
export function Rows({
  items,
  render,
  empty = "Nothing here yet.",
}: {
  items?: any[];
  render: (item: any) => ReactNode;
  empty?: string;
}) {
  return !items ? (
    <Status>Loading…</Status>
  ) : items.length ? (
    <div className="divide-y">
      {items.map((item, i) => (
        <section
          className="min-w-0 space-y-3 py-4 first:pt-0"
          key={item.id || item.ID || i}
        >
          {render(item)}
        </section>
      ))}
    </div>
  ) : (
    <p className="py-5 text-muted-foreground">{empty}</p>
  );
}
export function Link({ url, children }: { url?: string; children: ReactNode }) {
  const href =
    url?.startsWith("/") && !url.startsWith("//") ? url : safeURL(url);
  return href ? (
    <a
      className="underline decoration-muted-foreground/50 underline-offset-4"
      href={href}
    >
      {children}
    </a>
  ) : (
    <>{children}</>
  );
}
export function StateBadge({ value }: { value: string }) {
  const c = /fail|error/.test(value)
    ? "bg-red-50 text-red-800"
    : /block|pending|todo|warning/.test(value)
      ? "bg-amber-50 text-amber-900"
      : /done|success|complete|healthy|^ok$/.test(value)
        ? "bg-green-50 text-green-800"
        : "bg-blue-50 text-blue-800";
  return (
    <span className={`inline-flex rounded px-2 py-1 text-xs font-medium ${c}`}>
      {value}
    </span>
  );
}
