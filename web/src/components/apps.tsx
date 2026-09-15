import { useEffect, useState } from "react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Card } from "./ui/card";
import { NativeSelect } from "./ui/select";
import { PageHeading, Status } from "./layout";
import { json, mutate } from "../lib/api";
import catalogue from "../../../apps/catalog.json";
type App = {
  slug: string;
  name: string;
  description: string;
  author: string;
  tags: string;
  price: number;
  installs: number;
  can_edit: boolean;
  public: boolean;
};
export function AppsPage() {
  const params = new URLSearchParams(location.search);
  const [apps, setApps] = useState<App[]>(),
    [query, setQuery] = useState(""),
    [pricing, setPricing] = useState(params.get("pricing") || "all"),
    [tag, setTag] = useState(params.get("tag") || ""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  async function load() {
    setApps(await json<App[]>("/apps"));
  }
  useEffect(() => {
    load().catch((e) => setError(e.message));
  }, []);
  async function remove(app: App) {
    if (!confirm("Delete " + app.name + "?")) return;
    setBusy(true);
    setError("");
    try {
      await mutate("/apps/" + encodeURIComponent(app.slug) + "/delete", {});
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const tags = [
    ...new Set(
      apps?.flatMap((a) =>
        a.tags
          .split(",")
          .map((t) => t.trim())
          .filter(Boolean),
      ) || [],
    ),
  ].sort();
  const visible = apps?.filter(
    (a) =>
      (a.name + " " + a.description + " " + a.tags)
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (!tag ||
        a.tags
          .split(",")
          .map((t) => t.trim())
          .includes(tag)) &&
      (pricing === "all" || (pricing === "free" ? a.price === 0 : a.price > 0)),
  );
  return (
    <>
      <PageHeading
        title="Apps"
        actions={
          <Button asChild>
            <a href="/apps/new">New</a>
          </Button>
        }
      />
      <div className="mb-5 flex flex-wrap gap-2">
        <Input
          className="w-full max-w-xl"
          type="search"
          aria-label="Find apps"
          placeholder="Find an app"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <NativeSelect
          aria-label="Price"
          value={pricing}
          onChange={(e) => setPricing(e.target.value)}
        >
          <option value="all">All prices</option>
          <option value="free">Free</option>
          <option value="paid">Paid</option>
        </NativeSelect>
        {!!tags.length && (
          <NativeSelect
            aria-label="Tag"
            value={tag}
            onChange={(e) => setTag(e.target.value)}
          >
            <option value="">All tags</option>
            {tags.map((t) => (
              <option key={t}>{t}</option>
            ))}
          </NativeSelect>
        )}
      </div>
      {error && <Status error>{error}</Status>}
      {!apps && !error && <Status>Loading apps…</Status>}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {visible?.map((a) => (
          <Card key={a.slug}>
            <h2 className="break-words text-lg font-medium">
              <a href={"/apps/" + encodeURIComponent(a.slug)}>{a.name}</a>
            </h2>
            <p className="break-words text-muted-foreground">{a.description}</p>
            <div className="mt-auto flex flex-wrap justify-between gap-2 pt-2 text-sm text-muted-foreground">
              <span className="truncate">By {a.author}</span>
              <span>
                {a.public === false ? "Private · " : ""}
                {a.price ? a.price + " credits" : "Free"}
              </span>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" asChild>
                <a href={"/apps/" + encodeURIComponent(a.slug)}>Open</a>
              </Button>
              {a.can_edit && (
                <>
                  <Button variant="outline" asChild>
                    <a href={"/apps/" + encodeURIComponent(a.slug) + "/edit"}>
                      Edit
                    </a>
                  </Button>
                  <Button
                    disabled={busy}
                    variant="ghost"
                    onClick={() => remove(a)}
                  >
                    Delete
                  </Button>
                </>
              )}
            </div>
          </Card>
        ))}
      </div>
      <div className="mb-8 grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2 xl:grid-cols-3">
        {catalogue
          .filter(
            (a) =>
              a.name.toLowerCase().includes(query.toLowerCase()) &&
              pricing !== "paid" &&
              !tag,
          )
          .map((a) => (
            <section
              key={a.id}
              className="flex items-center justify-between gap-3 border-b py-3"
            >
              <h2 className="font-medium">
                <a href={a.path}>{a.name}</a>
              </h2>
              <Button asChild>
                <a href={a.path}>Open</a>
              </Button>
            </section>
          ))}
      </div>
      {visible?.length === 0 &&
        !catalogue.some(
          (a) =>
            a.name.toLowerCase().includes(query.toLowerCase()) &&
            pricing !== "paid" &&
            !tag,
        ) && (
          <p className="py-8 text-muted-foreground">
            {apps?.length ? "No apps match these filters." : "No apps yet."}
          </p>
        )}
    </>
  );
}
