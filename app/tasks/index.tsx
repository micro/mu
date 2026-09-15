import { useEffect, useState } from "react";
import {
  useData,
  json,
  mutate,
  Button,
  Status,
  Form,
  Rows,
  When,
  Read,
  Action,
  StateBadge,
  Link,
} from "../shared";
import { PageHeading } from "../../web/src/components/layout";
export function Tasks({ workspace = false }: {workspace?: boolean}) {
  const [filter, setFilter] = useState(
      new URLSearchParams(location.search).get("status") || "",
    ),
    [adding, setAdding] = useState(new URLSearchParams(location.search).get("view") === "new");
  const { data, error, refresh } = useData<any>(() => json("/tasks"));
  const all = data?.tasks || [];
  const id = new URLSearchParams(location.search).get("id");
  useEffect(() => {
    if (!all.some((t: any) => t.status === "doing")) return;
    const t = setInterval(refresh, 3000);
    return () => clearInterval(t);
  }, [data]);
  return (
    <>
      <PageHeading
        level={workspace ? 2 : 1}
        title="Tasks"
        actions={<Button onClick={() => setAdding(!adding)}>{workspace ? "New task" : "New"}</Button>}
      />
      {adding && (
        <div className="mb-6">
          <Form
            fields={[
              { name: "title", label: "What needs doing?", required: true },
              { name: "detail", label: "Detail", type: "textarea" },
              { name: "due", label: "Due", type: "datetime-local" },
              {
                name: "assign",
                label: "Assigned to",
                options: ["me", "agent"],
              },
            ]}
            submit={async (v) => {
              await mutate("/tasks", {
                ...v,
                due: v.due ? new Date(v.due).toISOString() : "",
              });
              setAdding(false);
              await refresh();
            }}
            label="Add"
          />
        </div>
      )}
      <div className="mb-5 flex flex-wrap gap-2">
        {["", "todo", "doing", "blocked", "failed", "done"].map((s) => (
          <Button
            key={s}
            aria-pressed={filter === s}
            variant={filter === s ? "secondary" : "outline"}
            onClick={() => setFilter(s)}
          >
            {s || "All"}
          </Button>
        ))}
      </div>
      {error && <Status error>{error}</Status>}
      <Rows
        items={
          data
            ? all
                .filter(
                  (t: any) =>
                    (!filter || t.status === filter) && (!id || t.id === id),
                )
                .sort((a: any, b: any) =>
                  String(b.updated || b.created).localeCompare(
                    String(a.updated || a.created),
                  ),
                )
            : undefined
        }
        render={(t) => (
          <>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 className="font-medium">
                <Link url={"/work?id=" + encodeURIComponent(t.id)}>
                  {t.title}
                </Link>
              </h2>
              <StateBadge value={t.status} />
            </div>
            <p className="text-sm text-muted-foreground">
              <When value={t.updated || t.created} />
              {t.due && !t.due.startsWith("0001") && (
                <>
                  {" "}
                  · Due <When value={t.due} />
                </>
              )}
            </p>
            {t.detail && (
              <details>
                <summary>Details</summary>
                <Read>{t.detail}</Read>
              </details>
            )}
            {t.result && <Read>{t.result}</Read>}
            {t.steps?.length > 0 && (
              <details>
                <summary>{t.steps.length} steps</summary>
                <Rows
                  items={t.steps}
                  render={(s) => (
                    <p>
                      {s.tool}: {s.detail}{" "}
                      <StateBadge value={s.ok ? "success" : "failed"} />
                    </p>
                  )}
                />
              </details>
            )}
            {t.thread && (
              <Link url={"/inbox?id=" + encodeURIComponent(t.thread)}>
                Conversation
              </Link>
            )}
            <div className="flex flex-wrap gap-2">
              {(t.status === "done"
                ? [["reopen", "Reopen"]]
                : [
                    ["done", "Done"],
                    ...(t.assignee === "agent"
                      ? [
                          ["unassign", "Take back"],
                          ...(t.status !== "doing" && !t.delivery
                            ? [
                                [
                                  "run",
                                  t.status === "failed" ||
                                  t.status === "blocked"
                                    ? "Retry"
                                    : "Run now",
                                ],
                              ]
                            : []),
                        ]
                      : [["assign", "Give to agent"]]),
                  ]
              ).map(([a, label]) => (
                <Action
                  key={a}
                  run={async () => {
                    await mutate(`/tasks/${encodeURIComponent(t.id)}/${a}`, {});
                    await refresh();
                  }}
                >
                  {label}
                </Action>
              ))}
              <Action
                danger
                run={async () => {
                  await mutate(`/tasks/${encodeURIComponent(t.id)}/delete`, {});
                  await refresh();
                }}
              >
                Delete
              </Action>
            </div>
          </>
        )}
      />
    </>
  );
}
