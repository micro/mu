import { useState } from "react";
import {
  useData,
  json,
  call,
  mutate,
  Button,
  Status,
  Search,
  Rows,
  When,
  Read,
  Action,
  Form,
  Link,
} from "./shared";
import { PageHeading, Pager } from "../web/src/components/layout";
const feedKeys: Record<string, string> = {
  news: "feed",
  video: "channels",
  social: "threads",
  stream: "entries",
  archive: "results",
  recall: "results",
  bookmarks: "items",
};
export function Reading({ name }: { name: string }) {
  if (
    location.pathname.startsWith("/blog/post") ||
    location.pathname === "/social/thread"
  )
    return <PostDetail social={name === "social"} />;
  return <Feed name={name} />;
}
function Feed({ name }: { name: string }) {
  const params = new URLSearchParams(location.search);
  const [q, setQ] = useState(params.get("q") || ""),
    [page, setPage] = useState(Number(params.get("page")) || 1),
    [adding, setAdding] = useState(params.has("new"));
  const [detail, setDetail] = useState<any>();
  const { data, error, refresh } = useData<any>(async () => {
    if (name === "recall")
      return json("/recall", {
        method: "POST",
        body: new URLSearchParams({ q }),
      });
    if (name === "news" && q)
      return json("/news", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query: q }),
      });
    if (name === "video" && q) return call("video", "search", { query: q });
    if (name === "bookmarks" && params.has("id"))
      return {
        items: [
          await json("/bookmarks?id=" + encodeURIComponent(params.get("id")!)),
        ],
        total: 1,
      };
    if (name === "bookmarks")
      return json("/bookmarks/search", {
        method: "POST",
        body: new URLSearchParams({
          query: q,
          offset: String((page - 1) * 20),
        }),
      });
    return json(
      `/${name}?page=${page}${q ? "&q=" + encodeURIComponent(q) : ""}${params.has("id") ? "&id=" + encodeURIComponent(params.get("id")!) : ""}`,
    );
  }, [name, q, page]);
  let items = Array.isArray(data)
    ? data
    : data?.results || data?.[feedKeys[name]];
  const article = name === "news" && params.has("id") ? data : detail;
  if (items && !Array.isArray(items))
    items = Object.values(items).flatMap((v: any) =>
      Array.isArray(v) ? v : v?.items || v?.videos || [],
    );
  if (data && !items) items = [];
  const localPage = ["news", "video", "stream", "recall", "archive"].includes(
    name,
  );
  const shown = localPage ? items?.slice((page - 1) * 20, page * 20) : items;
  return (
    <>
      <PageHeading
        title={
          name === "recall" ? "History" : name[0].toUpperCase() + name.slice(1)
        }
        actions={
          ["blog", "social", "bookmarks"].includes(name) ? (
            <Button onClick={() => setAdding(!adding)}>New</Button>
          ) : undefined
        }
      />
      <Search
        onSearch={(v) => {
          setQ(v);
          setPage(1);
        }}
        initial={q}
      />
      {adding && (
        <div className="mb-6">
          <Form
            fields={
              name === "bookmarks"
                ? [
                    { name: "url", label: "URL", type: "url", required: true },
                    { name: "title", label: "Title" },
                    { name: "note", label: "Notes", type: "textarea" },
                  ]
                : name === "blog"
                  ? [
                      { name: "title", label: "Title", required: true },
                      {
                        name: "content",
                        label: "Post",
                        type: "textarea",
                        required: true,
                      },
                      { name: "tags", label: "Tags" },
                    ]
                  : [
                      {
                        name: "content",
                        label: "Message",
                        type: "textarea",
                        required: true,
                      },
                    ]
            }
            submit={async (v) => {
              if (name === "bookmarks")
                await mutate("/bookmarks", { action: "add", ...v });
              else if (name === "blog")
                await mutate("/blog", { action: "create", ...v });
              else await mutate("/social", { action: "post", ...v });
              setAdding(false);
              await refresh();
            }}
          />
        </div>
      )}
      {error && <Status error>{error}</Status>}
      {article ? (
        <article className="space-y-4">
          <Button asChild>
            <a href={"/" + name}>Back</a>
          </Button>
          <h1 className="text-2xl font-semibold">{article.title}</h1>
          <Read>
            {article.summary ||
              article.content ||
              article.text ||
              article.description ||
              ""}
          </Read>
          {article.url && <Link url={article.url}>Source</Link>}
        </article>
      ) : (
        <>
          <Rows
            items={shown}
            render={(item) => (
              <>
                <div className="flex flex-wrap justify-between gap-2">
                  <h2 className="min-w-0 break-words font-medium">
                    {item.title ||
                      item.subject ||
                      item.author ||
                      item.service ||
                      item.number}
                  </h2>
                  <span className="text-sm text-muted-foreground">
                    <When
                      value={
                        item.published ||
                        item.posted_at ||
                        item.created_at ||
                        item.at ||
                        item.updated_at
                      }
                    />
                  </span>
                </div>
                {name === "video" && item.thumbnail && (
                  <a href={"/video?id=" + encodeURIComponent(item.id)}>
                    <img
                      className="aspect-video w-full max-w-xl rounded object-cover"
                      src={item.thumbnail}
                      alt={item.title || ""}
                      loading="lazy"
                    />
                  </a>
                )}
                <Read>
                  {item.description ||
                    item.summary ||
                    item.snippet ||
                    item.content ||
                    item.text ||
                    item.note ||
                    ""}
                </Read>
                <div className="flex flex-wrap gap-2">
                  {item.url && <Link url={item.url}>Open</Link>}
                  {name === "news" && (
                    <Button asChild>
                      <a href={"/news?id=" + encodeURIComponent(item.id)}>
                        Read
                      </a>
                    </Button>
                  )}
                  {name === "social" && (
                    <Button asChild>
                      <a
                        href={
                          "/social/thread?id=" + encodeURIComponent(item.id)
                        }
                      >
                        Replies
                      </a>
                    </Button>
                  )}
                  {name === "blog" && (
                    <Button asChild>
                      <a href={"/blog/post?id=" + encodeURIComponent(item.id)}>
                        Read
                      </a>
                    </Button>
                  )}
                  {name === "recall" && (
                    <Link
                      url={
                        "/inbox?id=" +
                        encodeURIComponent(
                          item.thread_id || item.thread || item.id,
                        )
                      }
                    >
                      Conversation
                    </Link>
                  )}
                  {["news", "video", "blog"].includes(name) && (
                    <Action
                      run={() =>
                        mutate("/bookmarks", {
                          action: "add",
                          ref: name + ":" + item.id,
                          url: item.url || "",
                          title: item.title || "",
                        })
                      }
                    >
                      Save
                    </Action>
                  )}
                  {name === "bookmarks" && (
                    <>
                      <Button asChild>
                        <a href={"/?bookmark=" + encodeURIComponent(item.id)}>
                          Discuss
                        </a>
                      </Button>
                      <details className="w-full">
                        <summary>Private note</summary>
                        <Form
                          initial={{ note: item.note || "" }}
                          fields={[
                            { name: "note", label: "Note", type: "textarea" },
                          ]}
                          label="Save note"
                          submit={async (v) => {
                            await mutate("/bookmarks", {
                              action: "note",
                              id: item.id,
                              ...v,
                            });
                            await refresh();
                          }}
                        />
                      </details>
                    </>
                  )}
                  {name === "bookmarks" && (
                    <Action
                      danger
                      run={async () => {
                        await mutate("/bookmarks", {
                          action: "delete",
                          id: item.id,
                        });
                        await refresh();
                      }}
                    >
                      Delete
                    </Action>
                  )}
                </div>
              </>
            )}
          />
          {items && (
            <Pager
              page={page}
              total={
                (localPage ? items.length : data?.total) ??
                (page - 1) * 20 + items.length + (items.length >= 20 ? 1 : 0)
              }
              size={20}
              onChange={setPage}
            />
          )}
        </>
      )}
    </>
  );
}

function PostDetail({ social }: { social: boolean }) {
  const params = new URLSearchParams(location.search),
    id = params.get("id") || location.pathname.split("/")[3] || "",
    base = social ? "/social/thread?id=" : "/blog/post?id=";
  const { data, error, refresh } = useData<any>(
    () => json(base + encodeURIComponent(id) + "&client=1"),
    [id, social],
  );
  const [editing, setEditing] = useState(params.has("edit"));
  const post = social ? data?.thread : data?.post;
  return (
    <>
      <PageHeading
        title={social ? "Social" : post?.title || "Blog"}
        actions={
          <Button asChild>
            <a href={social ? "/social" : "/blog"}>Back</a>
          </Button>
        }
      />
      {error && <Status error>{error}</Status>}
      {post && (
        <>
          {editing && !social && data.can_edit ? (
            <>
              <Form
                initial={{
                  ...post,
                  visibility: post.private ? "private" : "public",
                }}
                fields={[
                  { name: "title", label: "Title" },
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
                  await json(base + encodeURIComponent(id), {
                    method: "PATCH",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(v),
                  });
                  setEditing(false);
                  await refresh();
                }}
              />
              <Button onClick={() => setEditing(false)}>Cancel</Button>
            </>
          ) : (
            <article className="space-y-4">
              <p className="text-sm text-muted-foreground">
                {post.author} ·{" "}
                <When value={post.created_at || post.posted_at} />
              </p>
              <Read>{post.content}</Read>
              {data.can_edit && (
                <div className="flex gap-2">
                  <Button onClick={() => setEditing(true)}>Edit</Button>
                  <Action
                    danger
                    run={async () => {
                      await json(base + encodeURIComponent(id), {
                        method: "DELETE",
                        headers: { "Content-Type": "application/json" },
                      });
                      location.assign("/blog");
                    }}
                  >
                    Delete
                  </Action>
                </div>
              )}
            </article>
          )}
          <section className="mt-8 space-y-5">
            <h2 className="text-lg font-medium">
              {social ? "Replies" : "Comments"}
            </h2>
            <Rows
              items={data.comments || data.messages || []}
              render={(c) => (
                <>
                  <p className="text-sm text-muted-foreground">
                    {c.author} · <When value={c.created_at || c.posted_at} />
                  </p>
                  <Read>{c.content}</Read>
                </>
              )}
            />
            <Form
              fields={[
                {
                  name: "content",
                  label: social ? "Reply" : "Comment",
                  type: "textarea",
                  required: true,
                },
              ]}
              label="Post"
              submit={async (v) => {
                await mutate(
                  social
                    ? "/social/thread"
                    : "/blog/post/" + encodeURIComponent(id) + "/comment",
                  { ...v, reply_to: id },
                );
                await refresh();
              }}
            />
          </section>
        </>
      )}
    </>
  );
}
