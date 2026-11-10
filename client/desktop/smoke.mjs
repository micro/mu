import { _electron as electron } from "playwright-core";
import { createServer } from "node:http";
import { resolve } from "node:path";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import assert from "node:assert/strict";
let redirect,
  state,
  requests = 0;
const runtime = createServer(async (req, res) => {
  res.setHeader("Access-Control-Allow-Origin", "*");
  res.setHeader(
    "Access-Control-Allow-Headers",
    "Content-Type, Authorization, Micro-Protocol, Micro-Service, Micro-Endpoint, Micro-Id",
  );
  res.setHeader("Access-Control-Expose-Headers", "Micro-Protocol, Micro-Id");
  if (req.method === "OPTIONS") {
    res.writeHead(204);
    res.end();
    return;
  }
  const url = new URL(req.url, "http://localhost");
  let raw = "";
  for await (const part of req) raw += part;
  res.setHeader("Content-Type", "application/json");
  if (url.pathname === "/oauth/register") {
    redirect = JSON.parse(raw).redirect_uris[0];
    res.end('{"client_id":"desktop-test"}');
    return;
  }
  if (url.pathname === "/oauth/authorize") {
    state = url.searchParams.get("state");
    res.writeHead(302, { Location: redirect + "?code=one&state=" + state });
    res.end();
    return;
  }
  if (url.pathname === "/oauth/token") {
    assert.equal(new URLSearchParams(raw).get("redirect_uri"), redirect);
    res.end(
      '{"access_token":"desktop-token","token_type":"Bearer","expires_in":3600}',
    );
    return;
  }
  assert.equal(req.headers.authorization, "Bearer desktop-token");
  assert.equal(req.headers.cookie, undefined);
  res.setHeader("Micro-Protocol", "mucp/1");
  res.setHeader("Micro-Id", req.headers["micro-id"]);
  if (req.headers["micro-endpoint"] === "Assistant.ListAgents")
    res.end('{"agents":[{"id":"micro","slug":"micro","name":"Micro"}]}');
  else {
    requests++;
    res.end('{"thread":"one","text":"**Desktop reply**","content":[]}');
  }
});
await new Promise((ok) => runtime.listen(0, "127.0.0.1", ok));
const profile = await mkdtemp(resolve(tmpdir(), "mu-desktop-"));
const launch = () => electron.launch({ args: [resolve("."), "--user-data-dir=" + profile], timeout: 60000 });
let app = await launch();
try {
  await app.evaluate(({ shell, safeStorage }) => {
    safeStorage.isEncryptionAvailable = () => false;
    shell.openExternal = async (url) => {
      await fetch(url);
    };
  });
  let page = await app.firstWindow();
  await page.locator("#sign-in").waitFor();
  assert.equal(await page.locator("#runtime").count(), 0);
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await page
    .locator("#runtime")
    .fill("http://127.0.0.1:" + runtime.address().port);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.locator("#sign-in").waitFor();
  const previousOrigin = new URL(page.url()).origin;
  await app.close();
  app = await launch();
  page = await app.firstWindow();
  await page.locator("#sign-in").waitFor();
  assert.notEqual(new URL(page.url()).origin, previousOrigin);
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  assert.equal(await page.locator("#runtime").inputValue(), "http://127.0.0.1:" + runtime.address().port);
  await page.getByRole("link", { name: "Cancel", exact: true }).click();
  await app.evaluate(({ shell, safeStorage }) => {
    safeStorage.isEncryptionAvailable = () => false;
    shell.openExternal = async () => {};
  });
  await page.locator("#sign-in").click();
  await page.getByText("Complete sign-in in your browser, or try again.", { exact: true }).waitFor();
  assert.equal(await page.locator("#sign-in").isEnabled(), true);
  await app.evaluate(({ shell }) => { shell.openExternal = async (url) => { await fetch(url); }; });
  await page.locator("#sign-in").click();
  await page.locator("#send:not([disabled])").waitFor({ timeout: 20000 });
  await page.locator("#prompt").fill("Hello");
  await page.locator("#send").click();
  await page.locator(".answer").waitFor();
  assert.match(await page.locator(".answer").innerText(), /Desktop reply/);
  assert.equal(requests, 1);
  await mkdir("screenshots", { recursive: true });
  await page.screenshot({ path: "screenshots/desktop.png" });
} finally {
  await app.close();
  runtime.close();
  await rm(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
}
