import { ReadingActions } from "../reading-actions";
import { useState } from "react";
import {
  useData,
  json,
  call,
  Button,
  Search,
  Status,
  When,
  Link,
} from "../shared";
import { PageHeading, Pager } from "../../web/src/components/layout";
type Video = {
  id: string;
  title: string;
  thumbnail: string;
  channel: string;
  channel_id: string;
  category: string;
  published: string;
};
export function App() {
  const params = new URLSearchParams(location.search);
  const [query, setQuery] = useState(""),
    [category, setCategory] = useState(params.get("category") || ""),
    [page, setPage] = useState(1);
  const { data, error } = useData<any>(
    () => (query ? call("video", "search", { query }) : json("/video")),
    [query],
  );
  const channels = data?.channels || {};
  const categories = Object.keys(channels).sort();
  const items: Video[] = query
    ? data?.videos || data?.results || []
    : Object.entries(channels).flatMap(([key, ch]: [string, any]) =>
        (ch.videos || []).map((v: Video) => ({ ...v, category: key })),
      );
  const seen = new Set<string>(),
    covered = new Set<string>();
  const filtered = [...items]
    .sort(
      (a, b) =>
        b.published?.localeCompare(a.published) || a.id.localeCompare(b.id),
    )
    .filter((v) => {
      if (seen.has(v.id) || (category && v.category !== category)) return false;
      seen.add(v.id);
      if (category || query) return true;
      if (covered.has(v.category)) return false;
      covered.add(v.category);
      return true;
    });
  return (
    <>
      <PageHeading title="Video" />
      <Search
        onSearch={(q) => {
          setQuery(q);
          setCategory("");
          setPage(1);
        }}
      />
      <nav aria-label="Video categories" className="mb-6 flex flex-wrap gap-2">
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
            {c || "Latest"}
          </Button>
        ))}
      </nav>
      {error && <Status error>{error}</Status>}
      <div className="grid gap-x-5 gap-y-7 sm:grid-cols-2 lg:grid-cols-3">
        {filtered.slice((page - 1) * 9, page * 9).map((v) => (
          <article key={v.id} className="min-w-0 space-y-2">
            <a
              href={`/video?id=${encodeURIComponent(v.id)}`}
              className="block space-y-3"
            >
              <img
                src={
                  /^[A-Za-z0-9_-]{11}$/.test(v.id)
                    ? `/video/thumb?id=${v.id}`
                    : v.thumbnail
                }
                alt=""
                loading="lazy"
                className="aspect-video w-full rounded-lg bg-muted object-cover"
              />
              <h2 className="font-semibold leading-snug">{v.title}</h2>
            </a>
            <div className="space-y-1 text-sm text-muted-foreground">
              <Link
                url={
                  v.channel_id
                    ? `https://www.youtube.com/channel/${encodeURIComponent(v.channel_id)}`
                    : undefined
                }
              >
                {v.channel}
              </Link>
              <p>
                <When value={v.published} />
              </p>
            </div>
            <ReadingActions
              reference={`video_${v.id}`}
              href={`/video?id=${encodeURIComponent(v.id)}`}
            />
          </article>
        ))}
      </div>
      {data && !filtered.length && <p>No videos found.</p>}
      <Pager page={page} total={filtered.length} size={9} onChange={setPage} />
    </>
  );
}
