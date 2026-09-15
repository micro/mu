import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { ArrowUp, Mic, LocateFixed, LoaderCircle } from "lucide-react";
import { Button } from "./ui/button";
import { Textarea } from "./ui/textarea";
import { ResultView } from "./result";
import {
  csrf,
  guestKey,
  guestMessages,
  guestTurns,
  json,
  stream,
  type Message,
  type State,
} from "../lib/api";

export function Conversation({ state }: { state: State }) {
  const agentName = state.conversation?.agent_name || "Micro";
  const draftKey =
    "micro-draft:" +
    (state.account?.id || "guest") +
    (state.conversation?.attachment ? ":" + state.conversation.attachment : "");
  const [messages, setMessages] = useState<Message[]>(
      state.conversation?.messages || guestMessages,
    ),
    [draft, setDraft] = useState(
      () =>
        sessionStorage.getItem(draftKey) ||
        sessionStorage.getItem("micro-draft:guest") ||
        sessionStorage.getItem("mu_chat_draft:landing") ||
        "",
    ),
    [busy, setBusy] = useState(!!state.conversation?.pending),
    [status, setStatus] = useState(""),
    [error, setError] = useState(""),
    [listening, setListening] = useState(false),
    [location, setLocation] = useState<Record<string, unknown> | null>(null),
    [ready, setReady] = useState(false);
  const id = useRef(state.conversation?.id || ""),
    controller = useRef<AbortController | null>(null),
    recognition = useRef<any>(null),
    transcript = useRef<HTMLDivElement>(null),
    spacer = useRef<HTMLDivElement>(null),
    input = useRef<HTMLTextAreaElement>(null),
    follow = useRef(true),
    alive = useRef(true);
  const signedIn = !!state.account;
  const Speech =
    (window as any).SpeechRecognition ||
    (window as any).webkitSpeechRecognition;
  useEffect(() => {
    sessionStorage.setItem(draftKey, draft);
    if (state.account) sessionStorage.removeItem("micro-draft:guest");
  }, [draft, draftKey]);
  useLayoutEffect(() => {
    const box = transcript.current,
      pad = spacer.current;
    if (!box || !pad) return;
    const questions = box.querySelectorAll<HTMLElement>("[data-question]"),
      last = questions[questions.length - 1];
    if (!last) return;
    const top =
      last.getBoundingClientRect().top -
      box.getBoundingClientRect().top +
      box.scrollTop;
    pad.style.height = "0px";
    pad.style.height =
      Math.max(0, box.clientHeight - (box.scrollHeight - top) - 16) + "px";
    if (follow.current) box.scrollTo({ top: Math.max(0, top - 16) });
  }, [messages, busy]);
  useEffect(() => {
    const el = input.current;
    if (el) {
      el.style.height = "auto";
      el.style.height = Math.min(el.scrollHeight, 160) + "px";
    }
  }, [draft, draftKey]);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      controller.current?.abort();
      recognition.current?.abort();
    };
  }, []);
  async function recover() {
    const until = Date.now() + 600000;
    while (alive.current && Date.now() < until) {
      const pending = await json<any>(
        "/agent/pending?thread=" + encodeURIComponent(id.current),
      );
      if (!pending.waiting) {
        const next = await json<State>(
          "/?session=" + encodeURIComponent(id.current),
        );
        if (alive.current) {
          setMessages(next.conversation?.messages || []);
          setBusy(false);
          setStatus("");
          if (pending.error) setError(pending.error);
        }
        return;
      }
      await new Promise((r) => setTimeout(r, 3000));
    }
    throw Error("No answer came back. Please try again.");
  }
  useEffect(() => {
    let cancelled = false;
    async function init() {
      try {
        if (signedIn && guestTurns().length) {
          const turns = guestTurns();
          const saved = await json<{ id: string }>("/agent/handoff", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ turns }),
          });
          id.current = saved.id;
          const next = await json<State>(
            "/?session=" + encodeURIComponent(saved.id),
          );
          if (cancelled) return;
          setMessages(next.conversation?.messages || []);
          sessionStorage.removeItem(guestKey);
          ["hist", "conv", "ctx", "draft"].forEach((k) =>
            sessionStorage.removeItem("mu_chat_" + k + ":landing"),
          );
          history.replaceState(null, "", "/");
        }
        if (cancelled) return;
        setReady(true);
        if (state.conversation?.pending) {
          setStatus("Working…");
          await recover();
        }
      } catch (e) {
        if (!cancelled) {
          setError((e as Error).message);
          setBusy(false);
        }
      }
    }
    init();
    return () => {
      cancelled = true;
    };
  }, []);
  async function send(e: FormEvent) {
    e.preventDefault();
    const prompt = draft.trim();
    if (!prompt || busy || !ready) return;
    recognition.current?.stop();
    setListening(false);
    setError("");
    setBusy(true);
    setStatus("Working…");
    setDraft("");
    follow.current = true;
    setMessages((m) => [...m, { role: "person", text: prompt }]);
    controller.current = new AbortController();
    let completed = false;
    try {
      await stream(
        {
          prompt,
          agent: state.conversation?.agent || "",
          attachment: state.conversation?.attachment || "",
          context: {
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            ...(location ? { location } : {}),
          },
          history: signedIn ? [] : guestTurns().slice(-6),
          context_id: id.current,
          stream_text: true,
        },
        (ev) => {
          if (ev.type === "flow_id" && ev.thread) {
            id.current = ev.thread;
            if (signedIn) history.replaceState(null, "", "/");
          }
          if (ev.type === "stream_token") setStatus("Writing…");
          if (ev.type === "status" && ev.message) setStatus(ev.message);
          if (ev.type === "response") {
            completed = true;
            const message: Message = {
              role: "agent",
              text: ev.text || "",
              results: ev.results,
            };
            setMessages((m) => [...m, message]);
            if (!signedIn)
              sessionStorage.setItem(
                guestKey,
                JSON.stringify([
                  ...guestTurns(),
                  { prompt, answer: message.text, results: message.results },
                ]),
              );
            setBusy(false);
            setStatus("");
          }
          if (ev.type === "error") {
            completed = true;
            setError(
              ev.message ||
                ev.error ||
                "The agent could not complete this request.",
            );
            setDraft(prompt);
            setBusy(false);
            setStatus("");
          }
        },
        controller.current.signal,
      );
    } catch (e) {
      if (!alive.current) return;
      if (signedIn && id.current && !completed) {
        setStatus("Reconnecting…");
        try {
          await recover();
          return;
        } catch {}
      }
      setError((e as Error).message);
      setDraft(prompt);
    } finally {
      if (alive.current) {
        setBusy(false);
        setStatus("");
      }
    }
  }
  function dictate() {
    if (listening) {
      recognition.current?.stop();
      return;
    }
    const r = new Speech();
    recognition.current = r;
    const before = draft;
    r.lang = navigator.language;
    r.interimResults = true;
    r.onstart = () => setListening(true);
    r.onend = () => setListening(false);
    r.onerror = (e: any) => {
      setListening(false);
      setError(
        e.error === "not-allowed"
          ? "Microphone access was denied."
          : "Dictation stopped. Please try again.",
      );
    };
    r.onresult = (e: any) =>
      setDraft(
        (before ? before + " " : "") +
          Array.from(e.results)
            .map((r: any) => r[0].transcript)
            .join(""),
      );
    r.start();
  }
  function locate() {
    if (location) {
      setLocation(null);
      return;
    }
    navigator.geolocation.getCurrentPosition(
      (p) => {
        setLocation({
          latitude: Math.round(p.coords.latitude * 100) / 100,
          longitude: Math.round(p.coords.longitude * 100) / 100,
          accuracy_m: Math.max(1600, p.coords.accuracy),
          captured_at: new Date().toISOString(),
          source: "device",
        });
      },
      () =>
        setError("Location unavailable. You can type your location instead."),
      { timeout: 10000, maximumAge: 300000 },
    );
  }
  const empty = !messages.length && !busy;
  return (
    <div
      className={
        "flex min-h-0 flex-1 flex-col " +
        (empty && !signedIn ? "justify-center" : "")
      }
    >
      {state.conversation?.attachment_title && (
        <div className="mb-4 rounded-lg border p-3">
          <p className="font-medium">{state.conversation.attachment_title}</p>
          <p className="text-sm text-muted-foreground">
            This material accompanies your question.
          </p>
        </div>
      )}
      {empty && (
        <div
          className={
            "mb-6 text-center " +
            (signedIn ? "flex flex-1 flex-col justify-center" : "")
          }
        >
          <h1 className="text-3xl font-semibold tracking-tight">{agentName}</h1>
          <p className="mt-3 text-muted-foreground">A personal AI agent</p>
        </div>
      )}
      {!empty && (
        <div
          ref={transcript}
          onScroll={() => {
            const e = transcript.current;
            follow.current =
              !e || e.scrollHeight - e.scrollTop - e.clientHeight < 100;
          }}
          className="relative min-h-0 flex-1 chat-transcript overflow-y-auto overscroll-contain py-6"
          role="log"
          aria-label="Conversation"
        >
          {messages.map((m, i) => (
            <article
              key={m.id || i}
              data-question={m.role === "person" ? "" : undefined}
              className={
                m.role === "person"
                  ? "mb-6 whitespace-pre-wrap break-words font-medium"
                  : "mb-8"
              }
              aria-label={m.role === "person" ? "You" : agentName}
            >
              {m.role === "person" ? (
                m.text
              ) : (
                <>
                  <div className="markdown">
                    <Markdown remarkPlugins={[remarkGfm]}>{m.text}</Markdown>
                  </div>
                  {m.results?.map((r, j) => (
                    <ResultView key={j} item={r} signedIn={signedIn} />
                  ))}
                </>
              )}
            </article>
          ))}
          {busy && (
            <p
              className="mb-4 flex items-center gap-2 text-sm text-muted-foreground"
              role="status"
            >
              <LoaderCircle className="size-4 animate-spin" />
              {status || "Working…"}
            </p>
          )}
          <div ref={spacer} aria-hidden="true" />
        </div>
      )}
      <div className="relative shrink-0 pb-2">
        {error && (
          <div
            role="alert"
            className="absolute bottom-full mb-2 w-full rounded-md border bg-background p-3 text-sm"
          >
            {error}
            <Button
              variant="ghost"
              size="sm"
              className="ml-2"
              onClick={() => setError("")}
            >
              Dismiss
            </Button>
          </div>
        )}
        <form
          onSubmit={send}
          className="flex items-end gap-1 rounded-xl border border-input bg-background p-2 focus-within:ring-1 focus-within:ring-ring"
        >
          <Textarea
            ref={input}
            aria-label="Message Micro"
            placeholder="What do you need?"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
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
            rows={1}
            className="min-h-10 flex-1 resize-none border-0 bg-transparent px-2 py-2 text-base shadow-none focus-visible:ring-0"
          />
          {navigator.geolocation && (
            <Button
              type="button"
              variant={location ? "secondary" : "ghost"}
              size="icon"
              aria-pressed={!!location}
              aria-label={
                location
                  ? "Stop sharing approximate location"
                  : "Share approximate location"
              }
              title="Share approximate location with Micro and its model"
              onClick={locate}
            >
              <LocateFixed />
            </Button>
          )}
          {Speech && (
            <Button
              type="button"
              variant={listening ? "destructive" : "ghost"}
              size="icon"
              aria-pressed={listening}
              aria-label={listening ? "Stop dictation" : "Dictate"}
              onClick={dictate}
            >
              <Mic />
            </Button>
          )}
          <Button
            type="submit"
            size="icon"
            aria-label="Send"
            disabled={busy || !draft.trim() || !ready}
          >
            <ArrowUp />
          </Button>
        </form>
      </div>
    </div>
  );
}
