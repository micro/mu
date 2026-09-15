import { ReadingActions } from "../reading-actions";
import { useState } from "react";
import {
  useData,
  json,
  Button,
  Search,
  Status,
  When,
  Read,
  Link,
} from "../shared";
import { PageHeading, Pager } from "../../web/src/components/layout";
import { Badge } from "../../web/src/components/ui/badge";
type Article = {
  id: string;
  title: string;
  description: string;
  summary?: string;
  url: string;
  image: string;
  display_image?: string;
  category: string;
  posted_at: string;
  published: string;
};
function excerpt(value: string) {
  return value.replace(/<[^>]*>/g, "").slice(0, 240);
}
export function App() {
  const params = new URLSearchParams(location.search),
    id = params.get("id");
  const [query, setQuery] = useState(params.get("q") || ""),
    [category, setCategory] = useState(params.get("category") || ""),
    [page, setPage] = useState(1);
  const { data, error } = useData<any>(
    () =>
      id
        ? json(`/news?id=${encodeURIComponent(id)}`)
        : query
          ? json("/news", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ query }),
            })
          : json("/news"),
    [query, id],
  );
  const items: Article[] = data?.feed || data?.results || [];
  const sorted = [...items].sort((a, b) =>
    String(b.posted_at || b.published).localeCompare(
      String(a.posted_at || a.published),
    ),
  );
  const categories = [
    ...new Set(items.map((a) => a.category).filter(Boolean)),
  ].sort();
  const seen = new Set<string>();
  const filtered = sorted.filter((a) => {
    if (category) return a.category === category;
    if (query) return true;
    if (seen.has(a.category)) return false;
    seen.add(a.category);
    return true;
  });
  return (
    <>
      <PageHeading title="News" />
      {error && <Status error>{error}</Status>}
      {id ? (
        data && (
          <article className="mx-auto max-w-3xl space-y-4">
            <Link url="/news">Headlines</Link>
            <h2 className="text-2xl font-semibold">{data.title}</h2>
            {data.image && (
              <img
                src={data.image}
                alt=""
                className="max-h-80 w-full rounded-lg object-cover"
              />
            )}
            <Read>{data.summary || data.description || ""}</Read>
            <Link url={data.url}>Read source</Link>
          </article>
        )
      ) : (
        <>
          <Search
            initial={query}
            onSearch={(v) => {
              setQuery(v);
              setPage(1);
            }}
          />
          <nav
            className="mb-5 flex flex-wrap gap-2"
            aria-label="News categories"
          >
            {["", ...categories].map((c) => (
              <Button
                key={c}
                aria-pressed={c === category}
                onClick={() => {
                  setCategory(c);
                  setPage(1);
                }}
                className="capitalize aria-pressed:bg-foreground aria-pressed:text-background"
              >
                {c || "Headlines"}
              </Button>
            ))}
          </nav>
          <div className="divide-y">
            {filtered.slice((page - 1) * 20, page * 20).map((a) => (
              <article key={a.id} className="flex gap-4 py-5 first:pt-0">
                <div className="min-w-0 flex-1 space-y-2">
                  <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    {a.category && <Badge>{a.category}</Badge>}
                    <When value={a.posted_at || a.published} />
                  </div>
                  <h2 className="text-lg font-semibold leading-snug">
                    <a href={`/news?id=${encodeURIComponent(a.id)}`}>
                      {a.title}
                    </a>
                  </h2>
                  <p className="line-clamp-3 text-sm text-muted-foreground">
                    {excerpt(a.description || "")}
                  </p>
                  <Link url={a.url}>Source</Link>
                  <ReadingActions
                    reference={a.id}
                    href={`/news?id=${encodeURIComponent(a.id)}`}
                  />
                </div>
                {a.image && (
                  <img
                    src={a.display_image || a.image}
                    alt=""
                    loading="lazy"
                    className="size-24 shrink-0 rounded-lg object-cover sm:h-28 sm:w-40"
                  />
                )}
              </article>
            ))}
          </div>
          {data && !filtered.length && <p>No articles found.</p>}
          <Pager
            page={page}
            total={filtered.length}
            size={20}
            onChange={setPage}
          />
        </>
      )}
    </>
  );
}
