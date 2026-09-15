export type Result = {
  kind: string;
  id?: string;
  url?: string;
  title?: string;
  summary?: string;
  steps?: string[];
  shape?: { Lat: number; Lon: number }[];
};
export type Message = {
  id?: string;
  role: string;
  text: string;
  results?: Result[];
};
export type Conversation = {
  id: string;
  messages: Message[];
  pending: boolean;
  agent?: string;
  agent_name?: string;
  attachment?: string;
  attachment_title?: string;
};
export type Identity = { id: string; name: string; admin: boolean };
export type State = {
  account: Identity | null;
  csrf: string;
  conversation?: Conversation;
};
export function csrf() {
  return decodeURIComponent(
    document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)?.[1] || "",
  );
}
export async function json<T>(url: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(url, {
    ...init,
    cache: "no-store",
    headers: {
      Accept: "application/json",
      "X-CSRF-Token": csrf(),
      ...init.headers,
    },
  });
  if (res.redirected && new URL(res.url).pathname === "/login") {
    location.assign(
      "/login?redirect=" +
        encodeURIComponent(location.pathname + location.search),
    );
    throw Error("Please log in again.");
  }
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      const body = await res.json();
      message = body.error || message;
    } catch {}
    throw Error(message);
  }
  return res.json();
}
export function safeURL(value?: string) {
  try {
    const u = new URL(value || "");
    return ["https:", "http:"].includes(u.protocol) ? u.href : undefined;
  } catch {
    return undefined;
  }
}
export async function stream(
  body: unknown,
  onEvent: (event: any) => void,
  signal: AbortSignal,
) {
  const res = await fetch("/agent", {
    method: "POST",
    signal,
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      "X-Micro-Client": "web",
      "X-CSRF-Token": csrf(),
    },
    body: JSON.stringify(body),
  });
  if (!res.ok || !res.body) {
    let message =
      res.status === 402
        ? "You need more credits. Open Account to top up."
        : res.status === 429
          ? "Please wait a moment before trying again."
          : "Unable to send your message.";
    try {
      message = (await res.json()).error || message;
    } catch {}
    throw Error(message);
  }
  const reader = res.body.getReader(),
    decoder = new TextDecoder();
  let buffer = "",
    completed = false;
  try {
    while (true) {
      const chunk = await reader.read();
      buffer += decoder.decode(chunk.value, { stream: !chunk.done });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";
      for (const line of lines) {
        if (!line.startsWith("data:")) continue;
        const event = JSON.parse(line.slice(5));
        onEvent(event);
        if (event.type === "response" || event.type === "error")
          completed = true;
      }
      if (chunk.done) break;
    }
    if (!completed)
      throw Error(
        "Connection interrupted. Your message may still be processing.",
      );
  } finally {
    reader.releaseLock();
  }
}
export const guestKey = "micro-guest-conversation";
export function guestTurns(): {
  prompt: string;
  answer: string;
  results?: Result[];
}[] {
  try {
    return JSON.parse(
      sessionStorage.getItem(guestKey) ||
        sessionStorage.getItem("mu_chat_hist:landing") ||
        "[]",
    );
  } catch {
    return [];
  }
}
export function guestMessages(): Message[] {
  return guestTurns().flatMap((t) => [
    { role: "person", text: t.prompt },
    { role: "agent", text: t.answer, results: t.results },
  ]);
}

export async function mutate(url: string, values: Record<string, string>) {
  const res = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json", "X-CSRF-Token": csrf() },
    body: new URLSearchParams(values),
  });
  if (!res.ok) {
    let message = "Could not save";
    try {
      message = (await res.json()).error || message;
    } catch {}
    throw Error(message);
  }
  const redirect = new URL(res.url);
  if (redirect.searchParams.has("problem"))
    throw Error(redirect.searchParams.get("problem")!);
  if (redirect.searchParams.has("error"))
    throw Error(redirect.searchParams.get("error")!);
  if (redirect.pathname === "/login")
    throw Error("Your session expired. Please log in again.");
  return res;
}

// Data prepared by the owning Go handler for this document only.
export function initialData<T>(key = "page"): T | undefined {
  if (typeof document === "undefined") return undefined;
  try { return JSON.parse(document.getElementById("client-data")?.textContent || "{}")[key]; } catch { return undefined; }
}
