import test from "node:test";
import assert from "node:assert/strict";
import { renewSession, disconnect, loadSession } from "./auth.ts";
import { invoke, type Session } from "./protocol.ts";

function environment() {
  const values = new Map<string, string>();
  Object.assign(globalThis, {
    window: {},
    sessionStorage: {
      getItem: (k: string) => values.get(k) ?? null,
      setItem: (k: string, v: string) => values.set(k, v),
      removeItem: (k: string) => values.delete(k),
    },
  });
  return values;
}
const expired = (): Session => ({
  server: "https://runtime.example",
  token: "old",
  expires: Date.now() - 1000,
  client: "client",
  refresh: "refresh",
});
const renewed = () =>
  new Response(
    JSON.stringify({
      access_token: "new",
      refresh_token: "rotated",
      token_type: "Bearer",
      expires_in: 86400,
    }),
  );

test("expired session renews once before concurrent protocol reads", async () => {
  const values = environment(),
    session = expired();
  values.set("mu-web-session", JSON.stringify(session));
  assert.ok(loadSession());
  let renewals = 0,
    calls = 0;
  const original = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    if (String(url).endsWith("/oauth/token")) {
      renewals++;
      assert.equal(init?.credentials, "omit");
      assert.equal(
        new URLSearchParams(String(init?.body)).get("refresh_token"),
        "refresh",
      );
      await new Promise((resolve) => setTimeout(resolve, 10));
      return renewed();
    }
    calls++;
    const headers = new Headers(init?.headers);
    assert.equal(headers.get("Authorization"), "Bearer new");
    return new Response("{}", {
      headers: {
        "Micro-Protocol": "mucp/1",
        "Micro-Id": headers.get("Micro-Id")!,
        "Micro-Service": headers.get("Micro-Service")!,
        "Micro-Endpoint": headers.get("Micro-Endpoint")!,
        "Content-Type": "application/json",
      },
    });
  };
  try {
    await Promise.all([
      invoke(session, "assistant", "Home.Overview", {}),
      invoke(session, "assistant", "Inbox.List", {}),
    ]);
    assert.equal(renewals, 1);
    assert.equal(calls, 2);
    assert.equal(loadSession()?.refresh, "rotated");
  } finally {
    globalThis.fetch = original;
  }
});

test("network failure retains credentials and never sends the action", async () => {
  environment();
  const session = expired();
  const original = globalThis.fetch;
  globalThis.fetch = async () => {
    throw Error("offline");
  };
  try {
    await assert.rejects(
      invoke(session, "assistant", "Assistant.Submit", {}),
      /offline/,
    );
    assert.equal(session.refresh, "refresh");
  } finally {
    globalThis.fetch = original;
  }
});

test("disconnect during renewal cannot restore sign-in", async () => {
  const values = environment(),
    session = expired(),
    original = globalThis.fetch;
  let release!: () => void;
  globalThis.fetch = async () => {
    await new Promise<void>((resolve) => {
      release = resolve;
    });
    return renewed();
  };
  try {
    const pending = renewSession(session);
    disconnect();
    release();
    await assert.rejects(pending, /disconnected/);
    assert.equal(values.has("mu-web-session"), false);
  } finally {
    globalThis.fetch = original;
  }
});
