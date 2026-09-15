import { ReadingActions } from "../reading-actions";
import { useState } from "react";
import { Reading } from "../reading";
import {
  useData,
  json,
  mutate,
  Button,
  Search,
  Status,
  When,
  Form,
} from "../shared";
import { PageHeading, Pager } from "../../web/src/components/layout";
import { Badge } from "../../web/src/components/ui/badge";
type Post = {
  id: string;
  title: string;
  content: string;
  author: string;
  tags: string;
  private: boolean;
  created_at: string;
};
export function App() {
  if (location.pathname.startsWith("/blog/post"))
    return <Reading name="blog" />;
  return <Blog />;
}
function Blog() {
  const params = new URLSearchParams(location.search);
  const [query, setQuery] = useState(params.get("q") || ""),
    [page, setPage] = useState(1),
    [writing, setWriting] = useState(params.has("write") || params.has("new"));
  const { data, error, refresh } = useData<Post[]>(
    () =>
      json(
        `/blog?page=${page}${query ? "&q=" + encodeURIComponent(query) : ""}`,
      ),
    [page, query],
  );
  return (
    <div className="mx-auto max-w-3xl">
      <PageHeading
        title="Blog"
        actions={
          <Button onClick={() => setWriting(!writing)}>
            {writing ? "Cancel" : "New"}
          </Button>
        }
      />
      {writing ? (
        <Form
          label="Publish"
          fields={[
            { name: "title", label: "Title (optional)" },
            {
              name: "content",
              label: "Post",
              type: "textarea",
              required: true,
            },
            { name: "tags", label: "Tags" },
            {
              name: "visibility",
              label: "Visibility",
              options: ["public", "private"],
            },
          ]}
          submit={async (v) => {
            await mutate("/blog", { action: "create", ...v });
            setWriting(false);
            await refresh();
          }}
        />
      ) : (
        <Search
          initial={query}
          onSearch={(v) => {
            setQuery(v);
            setPage(1);
          }}
        />
      )}
      {error && <Status error>{error}</Status>}
      <div className="divide-y">
        {data?.map((post) => (
          <article key={post.id} className="space-y-3 py-6">
            <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
              <span>{post.author}</span>
              <span aria-hidden="true">·</span>
              <When value={post.created_at} />
              {post.private && <Badge>Private</Badge>}
            </div>
            <h2 className="text-xl font-semibold tracking-tight">
              <a href={`/blog/post?id=${encodeURIComponent(post.id)}`}>
                {post.title || post.content.split("\n")[0].slice(0, 100)}
              </a>
            </h2>
            <p className="line-clamp-3 leading-7 text-muted-foreground">
              {post.content
                .replace(/!\[[^\]]*\]\([^)]*\)/g, "")
                .replace(/[#*_`]/g, "")
                .slice(0, 320)}
            </p>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex flex-wrap gap-2">
                {post.tags
                  ?.split(",")
                  .filter(Boolean)
                  .map((tag) => (
                    <Badge key={tag}>{tag.trim()}</Badge>
                  ))}
              </div>
              <a
                className="text-sm underline underline-offset-4"
                href={`/blog/post?id=${encodeURIComponent(post.id)}`}
              >
                Read post
              </a>
            </div>
            <ReadingActions
              reference={post.id}
              href={`/blog/post?id=${encodeURIComponent(post.id)}`}
            />
          </article>
        ))}
      </div>
      {data && !data.length && (
        <p className="py-5 text-muted-foreground">No posts found.</p>
      )}
      <Pager
        page={page}
        total={
          (page - 1) * 20 + (data?.length || 0) + (data?.length === 20 ? 1 : 0)
        }
        size={20}
        onChange={setPage}
      />
    </div>
  );
}
