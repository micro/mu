import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { serve, secureOrigin } from "./server.mjs";
test("loopback app verifies OAuth state and prevents replay and asset escape", async () => {
  const root = await mkdtemp(join(tmpdir(), "mu-desktop-"));
  await writeFile(join(root, "index.html"), "Mu app");
  const callbacks = [];
  const s = await serve(root, (r) => callbacks.push(r));
  try {
    assert.equal(await (await fetch(s.url)).text(), "Mu app");
    assert.equal((await fetch(new URL("/", s.url))).status, 404);
    assert.equal((await fetch(s.url + "%2e%2e%2fpackage.json")).status, 404);
    assert.equal((await fetch(s.url, { method: "POST" })).status, 405);
    const state = "s".repeat(43),
      auth = new URL("https://runtime.example/oauth/authorize");
    auth.search = new URLSearchParams({
      state,
      redirect_uri: s.callback,
      response_type: "code",
      code_challenge_method: "S256",
      code_challenge: "c".repeat(43),
    }).toString();
    assert.equal(s.authorize(auth.href), auth.href);
    assert.equal(
      (
        await fetch(
          s.callback + "?code=one&state=" + encodeURIComponent("é".repeat(43)),
        )
      ).status,
      400,
    );
    assert.equal(
      (await fetch(s.callback + "?code=one&state=wrong")).status,
      400,
    );
    assert.equal(callbacks.length, 0);
    assert.equal(
      (await fetch(s.callback + "?code=one&state=" + state)).status,
      200,
    );
    assert.equal(callbacks[0].code, "one");
    assert.equal(
      (await fetch(s.callback + "?code=one&state=" + state)).status,
      400,
    );
    auth.searchParams.set("redirect_uri", "https://evil.example/");
    assert.throws(() => s.authorize(auth.href));
  } finally {
    s.close();
    await rm(root, { recursive: true, force: true });
  }
});
test("runtime validation rejects credentials and unsafe origins", () => {
  for (const u of ["https://micro.mu", "http://127.0.0.1:8080"])
    assert.equal(secureOrigin(u), u);
  for (const u of [
    "file:///tmp/a",
    "http://example.com",
    "https://user:secret@example.com",
    "https://example.com/path",
  ])
    assert.throws(() => secureOrigin(u));
});
