import { useState } from "react";
import {
  call,
  Button,
  Status,
  Search,
  Rows,
  Read,
  Link,
  Action,
  mutate,
} from "../shared";
import { PageHeading } from "../../web/src/components/layout";
export function WebSearch() {
  const [result, setResult] = useState<any>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [article, setArticle] = useState<any>();
  async function search(q: string) {
    setBusy(true);
    setError("");
    try {
      setResult(await call("web", "search", { query: q }));
      setArticle(undefined);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading title="Search" />
      <Search placeholder="Search the web" onSearch={(q) => void search(q)} />
      {busy && <Status>Searching…</Status>}
      {error && <Status error>{error}</Status>}
      {article ? (
        <article className="space-y-4">
          <Button onClick={() => setArticle(undefined)}>Back</Button>
          <h1 className="text-2xl font-semibold">{article.title}</h1>
          <Read>{article.content || article.text || ""}</Read>
          <Link url={article.url}>Source</Link>
        </article>
      ) : result?.results || result?.items ? (
        <Rows
          items={result.results || result.items}
          render={(r) => (
            <>
              <h2 className="font-medium">
                <Link url={r.url || r.link}>{r.title}</Link>
              </h2>
              <Read>{r.description || r.snippet || ""}</Read>
              <div className="flex flex-wrap gap-2">
                <Action
                  run={async () =>
                    setArticle(
                      await call("web", "fetch", { url: r.url || r.link }),
                    )
                  }
                >
                  Read
                </Action>
                <Action
                  run={() =>
                    mutate("/bookmarks", {
                      action: "add",
                      url: r.url || r.link,
                      title: r.title,
                    })
                  }
                >
                  Save
                </Action>
              </div>
            </>
          )}
        />
      ) : (
        result && <Read>{result.text || result.result || ""}</Read>
      )}
    </>
  );
}
