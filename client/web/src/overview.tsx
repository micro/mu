import { useEffect, useState } from "react";
import { invoke, type Agent, type Message, type Session } from "./protocol";
import { Turn, safeURL } from "./content";
import Markdown from "react-markdown";

type InboxItem = {
  id: string;
  subject: string;
  channel: string;
  unread: boolean;
};
type Service = {
  name: string;
  description: string;
  operations: { endpoint: string; description: string }[];
};
type Home = {
  agents: Agent[];
  inbox: InboxItem[];
  pinnedServices: string[];
  events: { title: string; start: string; location: string }[];
  brief: string;
  calendarPending: boolean;
};
export function Overview({
  session,
  view,
}: {
  session: Session;
  view: string;
}) {
  const [home, setHome] = useState<Home | null>(null),
    [items, setItems] = useState<InboxItem[]>([]),
    [services, setServices] = useState<Service[]>([]),
    [next, setNext] = useState(""),
    [status, setStatus] = useState("Loading…"),
    [selected, setSelected] = useState<InboxItem | null>(null),
    [messages, setMessages] = useState<Message[]>([]),
    [messagePage, setMessagePage] = useState(""),
    [busy, setBusy] = useState(false);
  async function load(page = "") {
    setBusy(true);
    try {
      if (view === "home")
        setHome(await invoke<Home>(session, "assistant", "Home.Overview", {}));
      if (view === "inbox") {
        const r = await invoke<{ items: InboxItem[]; nextPageToken: string }>(
          session,
          "assistant",
          "Inbox.List",
          { pageToken: page },
        );
        setItems((v) => (page ? [...v, ...r.items] : r.items));
        setNext(r.nextPageToken);
      }
      if (view === "services") {
        const r = await invoke<{ services: Service[]; nextPageToken: string }>(
          session,
          "assistant",
          "Services.List",
          { pageToken: page },
        );
        setServices((v) => (page ? [...v, ...r.services] : r.services));
        setNext(r.nextPageToken);
      }
      setStatus("");
    } catch (e) {
      setStatus(e instanceof Error ? e.message : "Unable to load.");
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void load();
    const id = new URLSearchParams(location.search).get("item");
    if (view === "inbox" && id)
      void read({ id, subject: "", channel: "", unread: false });
  }, [session, view]);
  async function read(item: InboxItem, page = "") {
    setBusy(true);
    try {
      const r = await invoke<{
        item: InboxItem;
        messages: Message[];
        nextPageToken: string;
      }>(session, "assistant", "Inbox.Read", { id: item.id, pageToken: page });
      setSelected(r.item);
      setMessages((v) => (page ? [...v, ...r.messages] : r.messages));
      setMessagePage(r.nextPageToken);
      setStatus("");
    } catch (e) {
      setStatus(e instanceof Error ? e.message : "Unable to read item.");
    } finally {
      setBusy(false);
    }
  }
  async function mark() {
    if (!selected) return;
    setBusy(true);
    try {
      const item = await invoke<InboxItem>(
        session,
        "assistant",
        "Inbox.MarkRead",
        { id: selected.id, unread: !selected.unread },
      );
      setSelected(item);
      setItems((v) => v.map((i) => (i.id === item.id ? item : i)));
      setStatus("");
    } catch (e) {
      setStatus(e instanceof Error ? e.message : "Unable to update item.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="section-stack">
      <h1>
        {view === "home" ? "Home" : view === "inbox" ? "Inbox" : "Services"}
      </h1>
      {status && <p role="status">{status}</p>}
      {view === "home" && home && (
        <>
          <h2>Agents</h2>
          <div className="collection-list">
            {home.agents.map((a) => (
              <a
                className="collection-item"
                key={a.id}
                href={"?agent=" + encodeURIComponent(a.id)}
              >
                <div>
                  <strong>{a.name}</strong>
                  <p className="text-muted">{a.description}</p>
                </div>
              </a>
            ))}
          </div>
          <p>
            <a href="?view=chats">All chats</a>
          </p>
          <h2>Inbox</h2>
          <p>
            <a href="?view=inbox">
              {home.inbox.length
                ? `${home.inbox.filter((i) => i.unread).length} unread among recent items`
                : "Your inbox is clear"}
            </a>
          </p>
          <h2>Your day</h2>
          {home.calendarPending && (
            <p className="text-muted">
              Calendar updates are pending. Showing cached events.
            </p>
          )}
          {home.events.length ? (
            <ul>
              {home.events.map((e, i) => (
                <li key={i}>
                  {e.start && new Date(e.start).toLocaleString()} — {e.title}
                  {e.location ? ` · ${e.location}` : ""}
                </li>
              ))}
            </ul>
          ) : (
            <p>No cached events.</p>
          )}
          {!!home.pinnedServices.length && (
            <>
              <h2>Pinned services</h2>
              <ul>
                {home.pinnedServices.map((s) => (
                  <li key={s}>
                    <a href={"?view=services#" + encodeURIComponent(s)}>{s}</a>
                  </li>
                ))}
              </ul>
              <a href="?view=services">Browse services</a>
            </>
          )}
          {home.brief && (
            <>
              <Turn
                name="Brief"
                server={session.server}
                message={{ role: "ROLE_AGENT", text: home.brief }}
              />
            </>
          )}
        </>
      )}
      {view === "inbox" &&
        (selected ? (
          <>
            <button
              onClick={() => {
                setSelected(null);
                setMessages([]);
              }}
            >
              Back to inbox
            </button>
            <h2>{selected.subject || "Untitled"}</h2>
            <button disabled={busy} onClick={() => void mark()}>
              {selected.unread ? "Mark read" : "Mark unread"}
            </button>
            {messages.map((m, i) => (
              <article className="collection-item" key={m.id || i}>
                <div className="message-body">
                  <Markdown
                    skipHtml
                    urlTransform={(url) => safeURL(url, session.server)}
                    components={{
                      a: (props) => (
                        <a
                          {...props}
                          target="_blank"
                          rel="noopener noreferrer"
                        />
                      ),
                    }}
                  >
                    {m.text}
                  </Markdown>
                </div>
              </article>
            ))}
            {messagePage && (
              <button
                disabled={busy}
                onClick={() => void read(selected, messagePage)}
              >
                More messages
              </button>
            )}
          </>
        ) : (
          <>
            <div className="collection-list">
              {items.map((i) => (
                <a
                  className="collection-item"
                  href={"?view=inbox&item=" + encodeURIComponent(i.id)}
                  key={i.id}
                  onClick={(e) => {
                    e.preventDefault();
                    if (!busy) void read(i);
                  }}
                >
                  <span>
                    {i.subject || "Untitled"}
                    {i.unread ? " · Unread" : ""}
                    <small> · {i.channel}</small>
                  </span>
                </a>
              ))}
            </div>
            {!items.length && !status && <p>Your inbox is clear.</p>}
          </>
        ))}
      {view === "services" && (
        <>
          <p className="text-muted">Ask your agent to use these services.</p>
          <div className="collection-list">
            {services.map((s) => (
              <article className="collection-item" id={s.name} key={s.name}>
                <div>
                  <strong>{s.name}</strong>
                  <p>{s.description}</p>
                </div>
              </article>
            ))}
          </div>
          {!services.length && !status && <p>No services available.</p>}
        </>
      )}
      {next && !selected && (
        <button disabled={busy} onClick={() => void load(next)}>
          More
        </button>
      )}
    </section>
  );
}
