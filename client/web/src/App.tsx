import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  call,
  ProtocolError,
  type Agent,
  type Message,
  type Session,
  type Thread,
} from "./protocol";
import {
  defaultRuntime,
  disconnect,
  signIn,
  hosted,
  accountURL,
  saveRuntime,
} from "./auth";
import { Turn } from "./content";
import { Voice } from "./voice";

function ConnectionSettings() {
  const [server, setServer] = useState(defaultRuntime),
    [error, setError] = useState("");
  return (
    <section className="section-stack">
      <h1>Settings</h1>
      <form
        className="form record-editor"
        onSubmit={(e) => {
          e.preventDefault();
          try {
            saveRuntime(server);
            location.assign(location.pathname);
          } catch (err) {
            setError(
              err instanceof Error ? err.message : "Invalid server address.",
            );
          }
        }}
      >
        <label htmlFor="runtime">Server address</label>
        <input
          id="runtime"
          type="url"
          required
          value={server}
          onChange={(e) => setServer(e.target.value)}
          autoComplete="url"
        />
        <p className="text-muted">
          Use micro.mu or connect to your own Mu server. Changing servers
          disconnects this app from the current account.
        </p>
        <div className="form-actions">
          <button>Save</button>
          <a href="./">Cancel</a>
        </div>
        <p role="status">{error}</p>
      </form>
    </section>
  );
}
function Connect({ error = "" }: { error?: string }) {
  const [status, setStatus] = useState(error),
    [busy, setBusy] = useState(false);
  return (
    <section className="section-stack">
      <h1>Welcome to Mu</h1>
      <p>Sign in to talk to your agents.</p>
      <div className="form-actions">
        <button
          id="sign-in"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            setStatus("Opening sign-in…");
            try {
              await signIn(defaultRuntime());
            } catch (err) {
              setStatus(
                err instanceof Error ? err.message : "Could not sign in.",
              );
              setBusy(false);
            }
          }}
        >
          Sign in
        </button>
        {hosted() && <a href={accountURL("signup")}>Create account</a>}
      </div>
      <p role="status">{status}</p>
    </section>
  );
}
export default function App({
  initial,
  error,
}: {
  initial: Session | null;
  error?: string;
}) {
  const [session, setSession] = useState(initial);
  const leave = () => {
    disconnect();
    if (session?.cookie) {
      location.assign(session.server + "/logout");
      return;
    }
    setSession(null);
  };
  return (
    <div className="page">
      <header>
        {session ? (
          <nav className="assistant-navigation" aria-label="Assistant">
            <a href="?view=agents">Agents</a>
            <a href="?view=chats">Chats</a>
          </nav>
        ) : (
          <span />
        )}
        <a className="brand" href="./">
          Mu
        </a>
        {session ? (
          <nav className="desktop-navigation">
            <a href="?view=account">Account</a>
          </nav>
        ) : (
          <nav className="desktop-navigation">
            <a href="?view=settings">Settings</a>
          </nav>
        )}
      </header>
      <main>
        {new URLSearchParams(location.search).get("view") === "settings" ? (
          <ConnectionSettings />
        ) : session ? (
          <Client session={session} disconnect={leave} />
        ) : (
          <Connect error={error} />
        )}
      </main>
    </div>
  );
}
function Client({
  session,
  disconnect,
}: {
  session: Session;
  disconnect: () => void;
}) {
  const query = new URLSearchParams(location.search),
    view = query.get("view") || "chat";
  const [agents, setAgents] = useState<Agent[]>([]),
    catalog = useRef<Agent[]>([]),
    [agent, setAgent] = useState<Agent | null>(null);
  const [messages, setMessages] = useState<Message[]>([]),
    [threads, setThreads] = useState<Thread[]>([]),
    [next, setNext] = useState("");
  const [thread, setThread] = useState(query.get("thread") || ""),
    [subject, setSubject] = useState(""),
    [prompt, setPrompt] = useState("");
  const [status, setStatus] = useState("Connecting…"),
    [busy, setBusy] = useState(true),
    [failedAuth, setFailedAuth] = useState(false);
  const epoch = useRef(0),
    timer = useRef<ReturnType<typeof setTimeout> | null>(null),
    responsePanel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    document.body.classList.add(
      view === "chat" ? "command-page" : "document-page",
    );
    return () =>
      document.body.classList.remove("command-page", "document-page");
  }, [view]);
  function fail(error: unknown) {
    if (error instanceof ProtocolError && error.status === 401) {
      setFailedAuth(true);
      setStatus("Your sign-in has expired. Sign in again to continue.");
      setBusy(false);
      return;
    }
    setStatus(error instanceof Error ? error.message : "Could not connect.");
    setBusy(false);
  }
  function updateThread(id: string, selected: Agent) {
    setThread(id);
    const q = new URLSearchParams({ agent: selected.id });
    if (id) q.set("thread", id);
    history.replaceState(null, "", "?" + q);
  }
  async function read(id: string, version: number) {
    try {
      let token = "",
        all: Message[] = [],
        pending = false;
      do {
        const result = await call(session, "ReadThread", {
          thread: id,
          pageSize: 100,
          pageToken: token,
        });
        if (version !== epoch.current) return;
        const selected = catalog.current.find(
          (a) => a.id === result.thread.agentId,
        );
        if (!selected)
          throw new ProtocolError("This agent is no longer available.", 404);
        setAgent(selected);
        setSubject(result.thread.subject);
        all.push(...result.messages);
        token = result.nextPageToken;
        pending = result.pending;
      } while (token);
      if (version !== epoch.current) return;
      setMessages(all);
      setStatus(pending ? "Working…" : "");
      setBusy(pending);
      if (pending) timer.current = setTimeout(() => read(id, version), 3000);
    } catch (error) {
      if (version !== epoch.current) return;
      if (
        error instanceof ProtocolError &&
        [400, 401, 403, 404].includes(error.status)
      ) {
        fail(error);
        return;
      }
      setStatus("Reconnecting…");
      timer.current = setTimeout(() => read(id, version), 5000);
    }
  }
  async function list(token = "") {
    setBusy(true);
    try {
      const result = await call(session, "ListThreads", {
        pageSize: 25,
        pageToken: token,
      });
      setThreads((items) =>
        token ? [...items, ...result.threads] : result.threads,
      );
      setNext(result.nextPageToken);
      setStatus("");
    } catch (error) {
      fail(error);
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    const version = epoch.current;
    (async () => {
      const result = await call(session, "ListAgents", {});
      if (version !== epoch.current) return;
      catalog.current = result.agents;
      setAgents(result.agents);
      const selected = query.get("agent");
      const current = selected
        ? result.agents.find((a) => a.id === selected || a.slug === selected)
        : result.agents.find((a) => a.id === "micro") || result.agents[0];
      if (!current) throw Error("This agent is not available on your account.");
      setAgent(current);
      if (view === "chats") {
        await list();
        return;
      }
      if (thread && view === "chat") {
        await read(thread, version);
        return;
      }
      setStatus("");
      setBusy(false);
    })().catch(fail);
    return () => {
      epoch.current++;
      if (timer.current) clearTimeout(timer.current);
      window.speechSynthesis?.cancel();
    };
  }, [session]);
  function newChat() {
    if (!agent) return;
    epoch.current++;
    if (timer.current) clearTimeout(timer.current);
    setBusy(false);
    setMessages([]);
    setSubject("");
    setStatus("");
    if (agent) updateThread("", agent);
  }
  async function send(event: FormEvent) {
    event.preventDefault();
    const text = prompt.trim();
    if (!agent || busy || !text) return;
    if (new TextEncoder().encode(text).length > 8000) {
      setStatus("Please shorten your message (maximum 8,000 UTF-8 bytes).");
      return;
    }
    window.speechSynthesis?.cancel();
    const version = epoch.current;
    setBusy(true);
    setPrompt("");
    setStatus("Working…");
    setMessages((items) => [...items, { role: "ROLE_PERSON", text }]);
    if (document.activeElement instanceof HTMLElement)
      document.activeElement.blur();
    try {
      const result = await call(session, "Ask", {
        agentId: agent.id,
        thread,
        prompt: text,
        mode: "ACCESS_MODE_PERSONAL",
      });
      if (version !== epoch.current) return;
      updateThread(result.thread, agent);
      setMessages((items) => [
        ...items,
        { role: "ROLE_AGENT", text: result.text, content: result.content },
      ]);
      setStatus("");
      setBusy(false);
    } catch (error) {
      if (version !== epoch.current) return;
      if (error instanceof ProtocolError && error.thread) {
        updateThread(error.thread, agent);
        if (error.status === 504) {
          await read(error.thread, version);
          return;
        }
      }
      fail(error);
      if (!(error instanceof ProtocolError))
        setStatus(
          "Connection lost. Check Chats for the reply before sending again.",
        );
    }
  }
  useEffect(() => {
    responsePanel.current?.lastElementChild?.scrollIntoView({
      block: "nearest",
    });
  }, [messages.length]);
  const notice = (
    <>
      {status && view !== "chat" && <p role="status">{status}</p>}
      {failedAuth && <button onClick={disconnect}>Sign in again</button>}
    </>
  );
  if (view === "account")
    return (
      <section>
        <h1>Account</h1>
        <p>Connected to {session.server}</p>
        <div className="collection-list">
          <a
            className="collection-item"
            href={session.server + "/account"}
            target="_blank"
            rel="noopener noreferrer"
          >
            Account and billing
          </a>
          <a
            className="collection-item"
            href={session.server + "/home"}
            target="_blank"
            rel="noopener noreferrer"
          >
            Open runtime
          </a>
          <a
            className="collection-item"
            href={session.server + "/download"}
            target="_blank"
            rel="noopener noreferrer"
          >
            Downloads
          </a>
        </div>
        <div className="form-actions">
          <a href="?view=settings">Settings</a>
          <button onClick={disconnect}>
            {session.cookie ? "Sign out" : "Disconnect"}
          </button>
        </div>
        <p className="text-muted">
          Disconnect clears this client’s sign-in. Revoke its access from your
          runtime account settings.
        </p>
        {notice}
      </section>
    );
  if (view === "agents")
    return (
      <section id="directory">
        <h1>Choose an agent</h1>
        {notice}
        <div className="collection-list">
          {agents.map((a) => (
            <a
              className="collection-item"
              href={"?agent=" + encodeURIComponent(a.id)}
              key={a.id}
            >
              <div>
                <strong>{a.name}</strong>
                <p className="text-muted">{a.description}</p>
              </div>
            </a>
          ))}
        </div>
      </section>
    );
  if (view === "chats")
    return (
      <section id="directory">
        <h1>Chats</h1>
        {notice}
        <div className="collection-list">
          {threads.map((t) => (
            <a
              className="collection-item"
              href={"?thread=" + encodeURIComponent(t.id)}
              key={t.id}
            >
              <div>
                <strong>{t.subject || "Untitled chat"}</strong>
                <p className="text-muted">
                  {agents.find((a) => a.id === t.agentId)?.name}
                </p>
              </div>
            </a>
          ))}
        </div>
        {!busy && !threads.length && (
          <p>Your conversations will appear here.</p>
        )}
        {next && (
          <button disabled={busy} onClick={() => list(next)}>
            More
          </button>
        )}
      </section>
    );
  const reply =
    [...messages].reverse().find((m) => m.role === "ROLE_AGENT")?.text || "";
  return (
    <section id="chat" className="assistant-workspace">
      <div className={"conversation" + (messages.length ? " is-active" : "")}>
        <div className="assistant-toolbar form-actions">
          <button id="new-chat" disabled={!agent} onClick={newChat}>
            New chat
          </button>
        </div>
        <h1>{subject || agent?.name || "Mu"}</h1>
        {!messages.length && <p className="text-muted">{agent?.description}</p>}
        <div id="responses" role="log" aria-label="Chat" ref={responsePanel}>
          {messages.map((m, i) => (
            <Turn
              key={m.id || i}
              message={m}
              name={agent?.name || "Assistant"}
              server={session.server}
            />
          ))}
        </div>
        <div className="prompt-panel">
          <form id="compose" onSubmit={send}>
            <label className="sr-only" htmlFor="prompt">
              Message
            </label>
            <div className="composer">
              <textarea
                id="prompt"
                rows={1}
                placeholder="What do you need?"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                onKeyDown={(e) => {
                  if (
                    e.key === "Enter" &&
                    !e.shiftKey &&
                    !e.nativeEvent.isComposing
                  ) {
                    e.preventDefault();
                    e.currentTarget.form?.requestSubmit();
                  }
                }}
                required
                maxLength={8000}
              />
              <button id="send" disabled={busy || failedAuth || !agent}>
                Send
              </button>
            </div>
            <Voice
              reply={reply}
              onDictation={setPrompt}
              onStatus={setStatus}
              thread={thread}
              busy={busy}
            />
            <p id="status" role="status">
              {status}
            </p>
            {failedAuth && (
              <button type="button" onClick={disconnect}>
                Sign in again
              </button>
            )}
          </form>
        </div>
      </div>
    </section>
  );
}
