import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runtimeOrigin, call, invoke, type Session } from "./protocol.ts";
import { challenge, validCallback } from "./auth.ts";
const session: Session = {
  server: "https://runtime.example",
  token: "scoped-token",
  expires: Date.now() + 10000,
};
test("only secure runtime origins and loopback development", () => {
  for (const value of [
    "https://runtime.example",
    "http://localhost:8080",
    "http://127.0.0.1:8080",
    "http://[::1]:8080",
  ])
    assert.equal(runtimeOrigin(value), value);
  for (const value of [
    "http://runtime.example",
    "https://user:pass@runtime.example",
    "https://runtime.example/path",
    "https://runtime.example?token=x",
    "javascript:alert(1)",
  ])
    assert.throws(() => runtimeOrigin(value));
});
test("OAuth S256 challenge matches RFC 7636 vector", async () => {
  assert.equal(
    await challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"),
    "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
  );
});
test("callback binds state, address and lifetime", () => {
  const p = {
    server: session.server,
    client: "one",
    redirect: "https://client.example/",
    verifier: "verifier",
    state: "state",
    created: 1000,
    query: "",
  };
  assert.ok(validCallback(p, "state", p.redirect, 2000));
  assert.equal(validCallback(p, "wrong", p.redirect, 2000), false);
  assert.equal(validCallback(p, "state", "https://evil.example/", 2000), false);
  assert.equal(validCallback(p, "state", p.redirect, 601001), false);
});
test("MUCP uses only explicit credentials and accepts the shared wire fixture", async () => {
  const previous = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    assert.equal(url, session.server + "/");
    assert.equal(init?.credentials, "omit");
    const headers = new Headers(init?.headers);
    assert.equal(headers.get("Authorization"), "Bearer scoped-token");
    assert.equal(headers.get("Micro-Endpoint"), "Assistant.Submit");
    return new Response(
      JSON.stringify({
        id: "operation-one",
        thread: "own-thread",
        state: "OPERATION_STATE_SUCCEEDED",
        result: JSON.parse(
          readFileSync(
            new URL(
              "../../../proto/testdata/ask-response.json",
              import.meta.url,
            ),
            "utf8",
          ),
        ),
      }),
      {
        headers: {
          "Micro-Protocol": "mucp/1",
          "Micro-Id": headers.get("Micro-Id")!,
        },
      },
    );
  };
  try {
    const response = await call(session, "Ask", {
      agentId: "micro",
      thread: "",
      prompt: "Hello",
      mode: "ACCESS_MODE_PERSONAL",
    });
    assert.ok(response.text);
    assert.ok(response.content.length);
  } finally {
    globalThis.fetch = previous;
  }
});
test("session transport keeps cookies and CSRF on the same origin only", async () => {
  const oldFetch = globalThis.fetch;
  Object.defineProperty(globalThis, "location", {
    configurable: true,
    value: { origin: session.server },
  });
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { cookie: "csrf_token=csrf-proof" },
  });
  let calls = 0;
  globalThis.fetch = async (_url, init) => {
    calls++;
    assert.equal(init?.credentials, "same-origin");
    const headers = new Headers(init?.headers);
    assert.equal(headers.get("Authorization"), null);
    assert.equal(headers.get("X-CSRF-Token"), "csrf-proof");
    return new Response('{"agents":[]}', {
      headers: {
        "Micro-Protocol": "mucp/1",
        "Micro-Id": headers.get("Micro-Id")!,
      },
    });
  };
  try {
    await call({ ...session, cookie: true, token: "" }, "ListAgents", {});
    await assert.rejects(
      call(
        { ...session, cookie: true, server: "https://other.example" },
        "ListAgents",
        {},
      ),
    );
    assert.equal(calls, 1);
  } finally {
    globalThis.fetch = oldFetch;
    Reflect.deleteProperty(globalThis, "location");
    Reflect.deleteProperty(globalThis, "document");
  }
});
test("all assistant exchanges follow the shared JSON contract", async () => {
  const cases = JSON.parse(
    readFileSync(
      new URL("../../../proto/testdata/exchanges.json", import.meta.url),
      "utf8",
    ),
  );
  const oldFetch = globalThis.fetch;
  try {
    for (const item of cases) {
      globalThis.fetch = async (_url, init) => {
        const headers = new Headers(init?.headers);
        assert.equal(
          headers.get("Micro-Endpoint"),
          "Assistant." + item.operation,
        );
        assert.equal(init?.redirect, "error");
        assert.deepEqual(JSON.parse(String(init?.body)), item.request);
        return new Response(JSON.stringify(item.response), {
          headers: {
            "Micro-Protocol": "mucp/1",
            "Micro-Id": headers.get("Micro-Id")!,
          },
        });
      };
      assert.deepEqual(
        await invoke(
          session,
          "assistant",
          "Assistant." + item.operation,
          item.request,
        ),
        item.response,
      );
    }
  } finally {
    globalThis.fetch = oldFetch;
  }
});
