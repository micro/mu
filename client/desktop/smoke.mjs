import { _electron as electron } from "playwright-core";
import { createServer } from "node:http";
import { resolve } from "node:path";
import { mkdir, mkdtemp, rm, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import assert from "node:assert/strict";
let redirect,
  state,
  requests = 0,
  renewals = 0;
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
    const registration = JSON.parse(raw);
    assert.equal(registration.client_name, "Mu Desktop");
    redirect = registration.redirect_uris[0];
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
    const form = new URLSearchParams(raw);
    if (form.get("grant_type") === "refresh_token") {
      assert.equal(form.get("client_id"), "desktop-test");
      assert.equal(form.get("refresh_token"), "desktop-refresh");
      renewals++;
      res.end(
        '{"access_token":"desktop-token","refresh_token":"rotated","token_type":"Bearer","expires_in":86400}',
      );
    } else {
      assert.equal(form.get("redirect_uri"), redirect);
      res.end(
        '{"access_token":"initial-token","refresh_token":"desktop-refresh","token_type":"Bearer","expires_in":1}',
      );
    }
    return;
  }
  assert.equal(req.headers.authorization, "Bearer desktop-token");
  assert.equal(req.headers.cookie, undefined);
  res.setHeader("Micro-Protocol", "mucp/1");
  res.setHeader("Micro-Id", req.headers["micro-id"]);
  if (req.headers["micro-endpoint"] === "Assistant.ListAgents")
    res.end('{"agents":[{"id":"micro","slug":"micro","name":"Micro"}]}');
  else if (req.headers["micro-endpoint"] === "Assistant.Submit") {
    requests++;
    const body = JSON.parse(raw);
    assert.ok(body.messageId);
    assert.equal(body.request.mode, "ACCESS_MODE_PERSONAL");
    res.end(
      '{"id":"operation-one","thread":"one","state":"OPERATION_STATE_QUEUED"}',
    );
  } else if (req.headers["micro-endpoint"] === "Assistant.ReadOperation") {
    assert.equal(JSON.parse(raw).id, "operation-one");
    res.end(
      '{"id":"operation-one","thread":"one","state":"OPERATION_STATE_SUCCEEDED","result":{"thread":"one","text":"**Desktop reply**","content":[]}}',
    );
  } else {
    res.writeHead(404);
    res.end('{"error":{"message":"Unsupported test operation"}}');
  }
});
await new Promise((ok) => runtime.listen(0, "127.0.0.1", ok));
const profile = await mkdtemp(resolve(tmpdir(), "mu-desktop-"));
const launch = () =>
  electron.launch({
    args: [resolve("."), "--user-data-dir=" + profile],
    timeout: 60000,
  });
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
  await page.reload();
  await page.locator("#sign-in").waitFor();
  await page.locator('[data-session-storage="temporary"]').waitFor();
  await mkdir("screenshots", { recursive: true });
  await page.screenshot({ path: "screenshots/desktop-sign-in.png" });
  assert.equal(await page.locator("#runtime").count(), 0);
  await app.evaluate(({ Menu }) =>
    Menu.getApplicationMenu().getMenuItemById("settings").click(),
  );
  await page.locator("#runtime").waitFor();
  await page
    .locator("#runtime")
    .fill("http://127.0.0.1:" + runtime.address().port);
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await page.locator("#sign-in").waitFor();
  const previousOrigin = new URL(page.url()).origin;
  const savedBounds = await app.evaluate(({ BrowserWindow }) => {
    const window = BrowserWindow.getAllWindows()[0];
    window.setBounds({ x: 50, y: 50, width: 900, height: 700 });
    return window.getNormalBounds();
  });
  await app.close();
  app = await launch();
  page = await app.firstWindow();
  await page.locator("#sign-in").waitFor();
  const restoredBounds = await app.evaluate(({ BrowserWindow }) =>
    BrowserWindow.getAllWindows()[0].getNormalBounds(),
  );
  assert.equal(restoredBounds.width, savedBounds.width);
  assert.equal(restoredBounds.height, savedBounds.height);
  assert.notEqual(new URL(page.url()).origin, previousOrigin);
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  assert.equal(
    await page.locator("#runtime").inputValue(),
    "http://127.0.0.1:" + runtime.address().port,
  );
  await page.getByRole("link", { name: "Cancel", exact: true }).click();
  await app.evaluate(({ shell, safeStorage }) => {
    safeStorage.isEncryptionAvailable = () => false;
    shell.openExternal = async () => {};
  });
  await writeFile(resolve(profile, "session.enc"), "stale encrypted session");
  await page.locator("#sign-in").click();
  await page
    .getByText("Complete sign-in in your browser, or try again.", {
      exact: true,
    })
    .waitFor();
  assert.equal(await page.locator("#sign-in").isEnabled(), true);
  await app.evaluate(({ shell }) => {
    shell.openExternal = async (url) => {
      await fetch(url);
    };
  });
  await page.locator("#sign-in").click();
  await page.locator("#send:not([disabled])").waitFor({ timeout: 20000 });
  await page.locator("#prompt").fill("Hello");
  await page.locator("#send").click();
  await page.locator(".answer").waitFor();
  assert.match(await page.locator(".answer").innerText(), /Desktop reply/);
  assert.equal(requests, 1);
  assert.equal(renewals, 1);
  await page.screenshot({ path: "screenshots/desktop.png" });
  // Temporary sign-in survives navigation/window recreation in this process,
  // but is never written to the vault when OS-backed encryption is unavailable.
  assert.equal(
    await page.evaluate(
      async () => (await window.muDesktop.loadSession())?.token,
    ),
    "desktop-token",
  );
  await assert.rejects(readFile(resolve(profile, "session.enc")), {
    code: "ENOENT",
  });
  await app.evaluate(({ Menu }) =>
    Menu.getApplicationMenu().getMenuItemById("account").click(),
  );
  await page.getByRole("heading", { name: "Account", exact: true }).waitFor();
  await page.locator('[data-session-storage="temporary"]').waitFor();
  await page.screenshot({ path: "screenshots/desktop-account.png" });
  await app.evaluate(({ Menu }) =>
    Menu.getApplicationMenu().getMenuItemById("new-chat").click(),
  );
  await page.locator("#send:not([disabled])").waitFor();
  assert.equal(await page.locator(".answer").count(), 0);
  assert.equal(requests, 1);
  await app.close();
  app = await launch();
  page = await app.firstWindow();
  await page.locator("#sign-in").waitFor();
} finally {
  await app.close();
  runtime.close();
  await rm(profile, {
    recursive: true,
    force: true,
    maxRetries: 5,
    retryDelay: 200,
  });
}
