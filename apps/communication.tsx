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
} from "./shared";
import { PageHeading, Pager } from "../web/src/components/layout";
import { Email } from "../web/src/components/email";
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
export function SMS() {
  const [adding, setAdding] = useState(false);
  const { data, error, refresh } = useData<any>(() => json("/sms"));
  return (
    <>
      <PageHeading
        title="SMS"
        actions={<Button onClick={() => setAdding(!adding)}>New</Button>}
      />
      {adding && (
        <div className="mb-6">
          <Form
            fields={[
              {
                name: "to",
                label: "Phone number",
                type: "tel",
                required: true,
              },
              {
                name: "channel",
                label: "Channel",
                options: ["sms", "whatsapp"],
              },
              {
                name: "text",
                label: "Message",
                type: "textarea",
                required: true,
              },
            ]}
            label="Send"
            submit={async (v) => {
              await call("sms", "send", v);
              setAdding(false);
              await refresh();
            }}
          />
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.messages}
        render={(m) => (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <span>
                {m.direction === "out" ? "To" : "From"} {m.number}
              </span>
              {m.status && <StateBadge value={m.status} />}
            </div>
            <Read>{m.text}</Read>
            <p className="text-sm text-muted-foreground">
              <When value={m.at} /> · {m.channel || "sms"}
            </p>
          </>
        )}
      />
    </>
  );
}
export function Notifications() {
  const [adding, setAdding] = useState(false);
  const { data, error, refresh } = useData<any>(() => json("/notify"));
  return (
    <>
      <PageHeading
        title="Notifications"
        actions={<Button onClick={() => setAdding(!adding)}>New</Button>}
      />
      <PushSettings publicKey={data?.public_key} refresh={refresh} />
      <p className="mb-4 text-muted-foreground">
        {data?.reachable
          ? "Notifications are enabled on your devices."
          : "Enable notifications on this device to receive updates."}{" "}
        <Link url="/account">Settings</Link>
      </p>
      {adding && (
        <div className="mb-6">
          <Form
            fields={[
              { name: "title", label: "Title", required: true },
              { name: "body", label: "Message", type: "textarea" },
            ]}
            label="Send to my devices"
            submit={async (v) => {
              await call("notify", "send", v);
              setAdding(false);
              await refresh();
            }}
          />
        </div>
      )}
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.sent}
        render={(n) => (
          <>
            <h2 className="font-medium">
              <Link url={n.url}>{n.title}</Link>
            </h2>
            <Read>{n.body || ""}</Read>
            <p className="text-sm text-muted-foreground">
              <When value={n.at || n.created} />
            </p>
          </>
        )}
      />
      <details className="mt-5">
        <summary>Devices</summary>
        <Rows
          items={data?.devices || []}
          render={(d) => (
            <p>
              {d.label} · {d.last || d.added}
            </p>
          )}
        />
      </details>
    </>
  );
}
export function People() {
  const [q, setQ] = useState("");
  const { data, error } = useData<any>(
    () => call("users", q ? "find" : "list", { query: q }),
    [q],
  );
  return (
    <>
      <PageHeading title="People" />
      <Search onSearch={setQ} placeholder="Find somebody" />
      {error && <Status error>{error}</Status>}
      <Rows
        items={data?.users}
        render={(u) => (
          <>
            <h2 className="font-medium">{u.account?.name || u.id}</h2>
            {u.profile?.online && <StateBadge value="online" />}
            <p>{u.status}</p>
            <div className="flex flex-wrap gap-2">
              <Button asChild>
                <a href={"/@" + encodeURIComponent(u.id)}>Profile</a>
              </Button>
              <Button asChild>
                <a href={"/chat?with=" + encodeURIComponent(u.id)}>Chat</a>
              </Button>
            </div>
          </>
        )}
      />
    </>
  );
}
export function Chat() {
  const id = new URLSearchParams(location.search).get("id");
  const { data, error } = useData<any>(
    () => json(id ? "/chat?id=" + encodeURIComponent(id) : "/chat"),
    [id],
  );
  return (
    <>
      <PageHeading
        title={data?.room?.title || "Chat"}
        actions={
          id ? (
            <Button asChild>
              <a href="/chat">Rooms</a>
            </Button>
          ) : undefined
        }
      />
      {error && <Status error>{error}</Status>}
      {id ? (
        <Room id={id} />
      ) : (
        <>
          <Rows
            items={data?.rooms}
            render={(r) => (
              <>
                <h2 className="font-medium">
                  <Link url={"/chat?id=" + encodeURIComponent(r.id || r.ID)}>
                    {r.title || r.Title || r.id || r.ID}
                  </Link>
                </h2>
                <p className="text-sm text-muted-foreground">
                  {r.participants ?? r.Participants ?? 0} here
                </p>
              </>
            )}
          />
          {data?.topics && (
            <div className="mt-5 flex flex-wrap gap-2">
              {Object.entries(data.topics).map(
                ([key, value]: [string, any]) => (
                  <Button key={key} asChild>
                    <a
                      href={
                        "/chat?id=chat_" +
                        encodeURIComponent(
                          typeof value === "string" ? value : key,
                        )
                      }
                    >
                      {typeof value === "string" ? value : key}
                    </a>
                  </Button>
                ),
              )}
            </div>
          )}
        </>
      )}
    </>
  );
}
function Room({ id }: { id: string }) {
  const [messages, setMessages] = useState<any[]>([]),
    [users, setUsers] = useState<string[]>([]),
    [draft, setDraft] = useState(""),
    [connected, setConnected] = useState(false),
    [error, setError] = useState("");
  const ws = useRef<WebSocket | null>(null);
  useEffect(() => {
    const socket = new WebSocket(
      `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/chat?id=${encodeURIComponent(id)}`,
    );
    ws.current = socket;
    socket.onopen = () => setConnected(true);
    socket.onclose = () => setConnected(false);
    socket.onerror = () => setError("Connection lost. Reload to reconnect.");
    socket.onmessage = (e) => {
      try {
        const m = JSON.parse(e.data);
        if (m.type === "user_list") {
          setUsers([...new Set<string>(m.users || [])]);
          return;
        }
        setMessages((items) => [...items, m]);
      } catch {}
    };
    return () => socket.close();
  }, [id]);
  return (
    <div className="flex h-[calc(100dvh-12rem)] min-h-80 flex-col gap-3">
      <p className="text-sm text-muted-foreground">
        Here: {users.join(", ") || "Nobody else"}
      </p>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <Rows
          items={messages}
          render={(m) => (
            <>
              <p className="text-sm text-muted-foreground">
                {m.username} · <When value={m.timestamp} />
              </p>
              <Read>{m.content || ""}</Read>
            </>
          )}
        />
      </div>
      {error && <Status error>{error}</Status>}
      <form
        className="flex items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!draft.trim() || !connected) return;
          ws.current?.send(JSON.stringify({ content: draft }));
          setDraft("");
        }}
      >
        <Textarea
          rows={2}
          aria-label="Message"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <Button disabled={!connected || !draft.trim()}>Send</Button>
      </form>
    </div>
  );
}

function PushSettings({
  publicKey,
  refresh,
}: {
  publicKey?: string;
  refresh: () => Promise<unknown>;
}) {
  const [sub, setSub] = useState<PushSubscription | null>(null),
    [status, setStatus] = useState("");
  const supported = "serviceWorker" in navigator && "PushManager" in window;
  useEffect(() => {
    if (supported)
      navigator.serviceWorker.ready
        .then((r) => r.pushManager.getSubscription())
        .then(setSub)
        .catch((e) => setStatus(e.message));
  }, []);
  async function post(action: string, body: any) {
    const r = await json<any>("/notify/" + action, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (r.error || r.ok === false)
      throw Error(r.error || "Notification request failed");
    return r;
  }
  return (
    <div className="mb-5 space-y-3">
      {supported ? (
        <div className="flex flex-wrap gap-2">
          <Action
            run={async () => {
              if (sub) {
                await post("unsubscribe", { endpoint: sub.endpoint });
                await sub.unsubscribe();
                setSub(null);
              } else {
                if (!publicKey)
                  throw Error(
                    "Push notifications are not configured on this instance",
                  );
                if ((await Notification.requestPermission()) !== "granted")
                  throw Error(
                    "Allow notifications in your browser settings, then try again.",
                  );
                const r = await navigator.serviceWorker.ready;
                const key = Uint8Array.from(
                  atob(publicKey.replace(/-/g, "+").replace(/_/g, "/")),
                  (c) => c.charCodeAt(0),
                );
                const subscription = await r.pushManager.subscribe({
                  userVisibleOnly: true,
                  applicationServerKey: key,
                });
                await post("subscribe", {
                  ...subscription.toJSON(),
                  label: navigator.userAgent,
                });
                setSub(subscription);
              }
              await refresh();
            }}
          >
            {sub ? "Turn off on this device" : "Enable on this device"}
          </Action>
          {sub && (
            <Action
              run={async () => {
                await post("test", { endpoint: sub.endpoint });
                setStatus("Test sent to this device.");
              }}
            >
              Test notification
            </Action>
          )}
        </div>
      ) : (
        <p className="text-muted-foreground">
          This browser does not support push notifications. On iPhone, install
          the app on your Home Screen first.
        </p>
      )}
      {status && <Status>{status}</Status>}
    </div>
  );
}
