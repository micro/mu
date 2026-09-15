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
      {!data && !error && <Status>Loading rooms…</Status>}
      {id ? (
        data && <Room key={id} id={id} />
      ) : data ? (
        <>
          <Rows
            items={data ? data.rooms || [] : undefined}
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
      ) : null}
    </>
  );
}
function Room({ id }: { id: string }) {
  const [messages, setMessages] = useState<any[]>([]),
    [autoScroll, setAutoScroll] = useState(() => localStorage.getItem("micro-room-autoscroll") !== "off"),
    [users, setUsers] = useState<string[]>([]),
    [draft, setDraft] = useState(""),
    [connected, setConnected] = useState(false),
    [historyReady, setHistoryReady] = useState(false),
    [error, setError] = useState("");
  const ws = useRef<WebSocket | null>(null);
  const transcript = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const restored = useRef(false);
  const identity = JSON.parse(document.getElementById("client-state")?.textContent || "{}").account?.id || "guest";
  const scrollKey = "micro-room-scroll:" + identity + ":" + id;
  useEffect(() => {
    const box = transcript.current;
    if (!box) return;
    if (!restored.current && historyReady) {
      restored.current = true;
      const saved = sessionStorage.getItem(scrollKey);
      if (saved !== null) { box.scrollTop = Number(saved); pinned.current = box.scrollHeight - box.scrollTop - box.clientHeight < 80; return; }
    }
    if (autoScroll && pinned.current) box.scrollTop = box.scrollHeight;
  }, [messages, autoScroll, historyReady]);
  useEffect(() => {
    const save = () => { if (transcript.current) sessionStorage.setItem(scrollKey, String(transcript.current.scrollTop)); };
    window.addEventListener("pagehide", save);
    return () => { save(); window.removeEventListener("pagehide", save); };
  }, [scrollKey]);
  useEffect(() => {
    let stopped = false;
    let retry: ReturnType<typeof setTimeout>;
    const connect = () => {
      if (stopped) return;
      const socket = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/chat?id=${encodeURIComponent(id)}`);
      ws.current = socket;
      socket.onopen = () => { setConnected(true); setError(""); };
      socket.onclose = () => {
        if(stopped) return;
        setConnected(false);
        setError("Connection lost. Reconnecting…");
        retry = setTimeout(connect, 2000);
      };
      socket.onerror = () => setError("Connection interrupted. Reconnecting…");
      socket.onmessage = (e) => {
        try {
          const m = JSON.parse(e.data);
          if (m.type === "user_list") {
            setUsers([...new Set<string>(m.users || [])]);
            setHistoryReady(true);
            return;
          }
          if (m.system && /^@\S+ (joined|left)$/.test(m.content || "")) return;
          const key = JSON.stringify([m.username, m.timestamp, m.content, m.system]);
          setMessages(items => items.some(item => item.key === key) ? items : [...items, {...m,key}]);
        } catch {}
      };
    };
    connect();
    return () => {stopped = true; clearTimeout(retry); ws.current?.close();};
  }, [id]);
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
        <p>Here: {users.join(", ") || "Nobody else"}</p>
        <label className="flex items-center gap-2"><input type="checkbox" checked={autoScroll} onChange={e => {setAutoScroll(e.target.checked); localStorage.setItem("micro-room-autoscroll", e.target.checked ? "on" : "off"); if(e.target.checked) pinned.current = true;}} />Auto-scroll</label>
      </div>
      <div ref={transcript} role="log" aria-label="Chat messages" className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain" onScroll={() => {const box=transcript.current!; pinned.current=box.scrollHeight-box.scrollTop-box.clientHeight<80;}}>

        <Rows
          items={messages}
          render={(m) => (
            <>
              <p className="text-sm text-muted-foreground">
                {m.username} · <When value={m.timestamp} />
              </p>
              {m.is_llm ? <Read>{m.content || ""}</Read> : <p className="whitespace-pre-wrap break-words leading-relaxed">{m.content || ""}</p>}
            </>
          )}
        />
      </div>
      {error && <Status error>{error}</Status>}
      <form
        className="flex shrink-0 items-end gap-2 pb-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!draft.trim() || !connected) return;
          pinned.current = true;
          ws.current?.send(JSON.stringify({ content: draft }));
          setDraft("");
        }}
      >
        <Textarea
          rows={1}
          className="min-h-9 max-h-36 resize-none text-base"
          onKeyDown={e => {if(e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {e.preventDefault(); e.currentTarget.form?.requestSubmit();}}}
          aria-label="Message"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <Button disabled={!connected || !draft.trim()}>Send</Button>
      </form>
    </div>
  );
}
