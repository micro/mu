// JSON representation of proto/assistant/v1/assistant.proto. Unknown content
// remains usable through its required fallback; execution belongs to the runtime.
export interface Agent {
  id: string;
  name: string;
  slug: string;
  description: string;
}
export interface Thread {
  id: string;
  agentId: string;
  subject: string;
  started?: string;
  updated?: string;
}
export interface Content {
  fallback: string;
  source?: { title: string; url: string; summary: string };
  resource?: {
    kind: string;
    id: string;
    title: string;
    url: string;
    summary: string;
  };
  video?: {
    provider: string;
    id: string;
    url: string;
    title: string;
    audioOnlyAllowed: boolean;
  };
  audio?: { url: string; title: string; mimeType: string };
  image?: { url: string; description: string };
  route?: { shape: { latitude: number; longitude: number }[]; steps: string[] };
}
export interface Message {
  id?: string;
  role: string;
  text: string;
  content?: Content[];
  at?: string;
}
interface Operations {
  ListAgents: { request: Record<string, never>; response: { agents: Agent[] } };
  ListThreads: {
    request: { agentId?: string; pageSize?: number; pageToken?: string };
    response: { threads: Thread[]; nextPageToken: string };
  };
  ReadThread: {
    request: { thread: string; pageSize?: number; pageToken?: string };
    response: {
      thread: Thread;
      messages: Message[];
      nextPageToken: string;
      pending: boolean;
    };
  };
  Ask: {
    request: {
      agentId: string;
      thread: string;
      prompt: string;
      mode: "ACCESS_MODE_PERSONAL";
      voice?: boolean;
    };
    response: {
      thread: string;
      agentId: string;
      text: string;
      content: Content[];
    };
  };
}
export interface Session {
  server: string;
  token: string;
  expires: number;
  cookie?: boolean;
}
export class ProtocolError extends Error {
  constructor(
    message: string,
    public status = 0,
    public thread = "",
  ) {
    super(message);
  }
}
export function runtimeOrigin(value: string): string {
  const u = new URL(value);
  if (
    u.username ||
    u.password ||
    u.search ||
    u.hash ||
    (u.pathname !== "/" && u.pathname !== "")
  )
    throw Error("Enter a runtime origin, without a path or credentials.");
  if (
    u.protocol !== "https:" &&
    !(
      u.protocol === "http:" &&
      ["localhost", "127.0.0.1", "[::1]"].includes(u.hostname)
    )
  )
    throw Error("Use HTTPS, or HTTP on localhost for development.");
  return u.origin;
}
export async function call<K extends keyof Operations>(
  session: Session,
  operation: K,
  body: Operations[K]["request"],
): Promise<Operations[K]["response"]> {
  if (operation === "Ask") {
    type Receipt = {
      id: string;
      thread: string;
      state: string;
      result?: Operations["Ask"]["response"];
      error?: string;
    };
    let receipt = await invoke<Receipt>(
      session,
      "assistant",
      "Assistant.Submit",
      { messageId: crypto.randomUUID(), request: body },
    );
    const deadline = Date.now() + 120000;
    for (;;) {
      if (receipt.state === "OPERATION_STATE_SUCCEEDED" && receipt.result)
        return receipt.result as Operations[K]["response"];
      if (
        ["OPERATION_STATE_FAILED", "OPERATION_STATE_INTERRUPTED"].includes(
          receipt.state,
        )
      )
        throw new ProtocolError(
          receipt.error ||
            "Execution stopped. Check Chats before trying again.",
          500,
          receipt.thread,
        );
      if (Date.now() >= deadline)
        throw new ProtocolError(
          "Your request is still running. Open Chats to recover the reply.",
          504,
          receipt.thread,
        );
      await new Promise((resolve) => setTimeout(resolve, 1000));
      try {
        receipt = await invoke<Receipt>(
          session,
          "assistant",
          "Assistant.ReadOperation",
          { id: receipt.id },
        );
      } catch (e) {
        throw new ProtocolError(
          e instanceof Error ? e.message : "Connection lost. Check Chats.",
          e instanceof ProtocolError ? e.status : 0,
          receipt.thread,
        );
      }
    }
  }
  return invoke(session, "assistant", "Assistant." + operation, body);
}
export async function invoke<T>(
  session: Session,
  service: string,
  endpoint: string,
  body: unknown,
): Promise<T> {
  if (session.cookie && runtimeOrigin(session.server) !== location.origin)
    throw new ProtocolError(
      "Session cookies cannot be used with another server.",
    );
  const id = crypto.randomUUID();
  const credentials: Record<string, string> = session.cookie
    ? {
        "X-CSRF-Token": decodeURIComponent(
          document.cookie
            .split("; ")
            .find((c) => c.startsWith("csrf_token="))
            ?.slice(11) || "",
        ),
      }
    : { Authorization: "Bearer " + session.token };
  const response = await fetch(runtimeOrigin(session.server) + "/", {
    method: "POST",
    credentials: session.cookie ? "same-origin" : "omit",
    cache: "no-store",
    redirect: "error",
    signal: AbortSignal.timeout(endpoint === "Assistant.Ask" ? 135000 : 20000),
    headers: {
      "Content-Type": "application/json",
      ...credentials,
      "Micro-Protocol": "mucp/1",
      "Micro-Service": service,
      "Micro-Endpoint": endpoint,
      "Micro-Id": id,
    },
    body: JSON.stringify(body),
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok)
    throw new ProtocolError(
      data.error?.message ||
        (typeof data.error === "string"
          ? data.error
          : "The runtime could not complete the request."),
      response.status,
      response.headers.get("Micro-Thread") || "",
    );
  if (
    response.headers.get("Micro-Protocol") !== "mucp/1" ||
    response.headers.get("Micro-Id") !== id
  )
    throw new ProtocolError(
      "This runtime does not support the client’s MUCP version.",
    );
  return data;
}
