import { useEffect, useRef, useState } from "react";
import {
  useData,
  json,
  mutate,
  call,
  Button,
  Status,
  Form,
  Search,
  Rows,
  When,
  Read,
  Action,
  StateBadge,
  Link,
  Textarea,
} from "../shared";
import { PageHeading, Pager } from "../../web/src/components/layout";
import { Email } from "../../web/src/components/email";
export function Mail() {
  const params = new URLSearchParams(location.search),
    [view, setView] = useState(params.get("view") || "inbox"),
    [id, setID] = useState(params.get("id") || ""),
    [q, setQ] = useState(""),
    [page, setPage] = useState(1),
    [compose, setCompose] = useState<any>(
      params.get("view") === "compose" ? {} : undefined,
    );
  const { data, error, refresh } = useData<any>(
    () =>
      json(
        "/mail?client=1&view=" +
          encodeURIComponent(view) +
          "&id=" +
          encodeURIComponent(id),
      ),
    [view, id],
  );
  const items = (data?.items || []).filter((m: any) =>
    (m.subject + " " + m.from + " " + m.snippet)
      .toLowerCase()
      .includes(q.toLowerCase()),
  );
  function open(id: string) {
    setID(id);
    history.replaceState(
      null,
      "",
      id ? "/mail?id=" + encodeURIComponent(id) : "/mail?view=" + view,
    );
  }
  return (
    <>
      <PageHeading
        title="Mail"
        actions={<Button onClick={() => setCompose({})}>New</Button>}
      />
      <p className="mb-4 text-sm text-muted-foreground">{data?.address}</p>
      <div className="mb-5 flex flex-wrap gap-2">
        {["inbox", "sent", "outbox", "filtered"].map((v) => (
          <Button
            key={v}
            aria-pressed={view === v}
            onClick={() => {
              setView(v);
              setID("");
              setPage(1);
              setCompose(undefined);
              history.replaceState(null, "", "/mail?view=" + v);
            }}
          >
            {v === "filtered" ? "Spam" : v[0].toUpperCase() + v.slice(1)}
          </Button>
        ))}
      </div>
      {error && <Status error>{error}</Status>}
      {compose ? (
        <>
          <Form
            key={compose.reply_to || "new"}
            initial={compose}
            fields={[
              { name: "to", label: "To", required: true },
              { name: "subject", label: "Subject", required: true },
              {
                name: "body",
                label: "Message",
                required: true,
                type: "textarea",
              },
            ]}
            label="Send"
            submit={async (v) => {
              await json("/mail", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  ...v,
                  reply_to: compose.reply_to || "",
                }),
              });
              setCompose(undefined);
              await refresh();
            }}
          />
          <Button className="mt-3" onClick={() => setCompose(undefined)}>
            Cancel
          </Button>
        </>
      ) : id ? (
        <>
          <Button className="mb-4" onClick={() => open("")}>
            All mail
          </Button>
          <Rows
            items={data?.items}
            render={(m) => (
              <>
                <h2 className="text-xl font-medium">{m.subject}</h2>
                <p className="text-sm text-muted-foreground">
                  {m.from} → {m.to} · <When value={m.created} />
                </p>
                <Email html={m.html} />
                <div className="flex flex-wrap gap-2">
                  <Button
                    onClick={() =>
                      setCompose({
                        to: m.recipient,
                        subject: /^re:/i.test(m.subject)
                          ? m.subject
                          : "Re: " + m.subject,
                        reply_to: m.reply_to,
                      })
                    }
                  >
                    Reply
                  </Button>
                  <Button asChild>
                    <a
                      href={
                        "/mail?action=view_raw&id=" + encodeURIComponent(m.id)
                      }
                    >
                      View raw
                    </a>
                  </Button>
                  {m.attachment_name && (
                    <Button asChild>
                      <a
                        href={
                          "/mail?action=download_attachment&id=" +
                          encodeURIComponent(m.id)
                        }
                      >
                        {m.attachment_name}
                      </a>
                    </Button>
                  )}
                  <Action
                    danger
                    run={async () => {
                      await mutate("/mail", { _method: "DELETE", id: m.id });
                      open("");
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
      ) : (
        <>
          <Search
            placeholder="Search this mailbox"
            onSearch={(v) => {
              setQ(v);
              setPage(1);
            }}
          />
          <Rows
            items={data ? items.slice((page - 1) * 20, page * 20) : undefined}
            render={(m) => (
              <>
                <button
                  className="w-full text-left"
                  onClick={() => view !== "outbox" && open(m.id)}
                >
                  <span className="block break-words font-medium">
                    {m.subject}
                  </span>
                  <span className="block text-sm text-muted-foreground">
                    {m.from || m.recipients?.join(", ")} ·{" "}
                    <When value={m.created} />
                  </span>
                  <span className="mt-2 line-clamp-2 block break-words text-muted-foreground">
                    {m.snippet || m.last_error}
                  </span>
                </button>
                {view === "filtered" && (
                  <>
                    <p>{m.spam_reasons?.join(", ")}</p>
                    <div className="flex gap-2">
                      <Action
                        run={async () => {
                          await mutate("/mail?view=filtered", {
                            action: "not_spam",
                            msg_id: m.id,
                          });
                          await refresh();
                        }}
                      >
                        Not spam
                      </Action>
                      <Action
                        danger
                        run={async () => {
                          await mutate("/mail?view=filtered", {
                            action: "delete_spam",
                            msg_id: m.id,
                          });
                          await refresh();
                        }}
                      >
                        Delete
                      </Action>
                    </div>
                  </>
                )}
                {view === "outbox" && (
                  <>
                    <p>
                      {m.pending ? "Queued" : "Needs attention"} · {m.attempts}{" "}
                      attempts
                    </p>
                    {!m.pending && (
                      <div className="flex gap-2">
                        {["retry", "discard"].map((action) => (
                          <Action
                            key={action}
                            danger={action === "discard"}
                            run={async () => {
                              await mutate("/mail?view=outbox", {
                                action,
                                id: m.id,
                              });
                              await refresh();
                            }}
                          >
                            {action === "retry" ? "Retry" : "Discard"}
                          </Action>
                        ))}
                      </div>
                    )}
                  </>
                )}
              </>
            )}
          />
          {data && (
            <Pager
              page={page}
              total={items.length}
              size={20}
              onChange={setPage}
            />
          )}
        </>
      )}
    </>
  );
}
