const assert = require("node:assert/strict"),
  fs = require("node:fs");
const { chromium } = require(process.env.MU_PLAYWRIGHT_MODULE || "playwright");
(async () => {
  const input = JSON.parse(fs.readFileSync(0, "utf8"));
  const browser = await chromium.launch({
    executablePath: process.env.MU_LAYOUT_BROWSER,
    headless: true,
    args: ["--no-sandbox", "--disable-dev-shm-usage"],
  });
  try {
    const context = await browser.newContext();
    await context.addCookies([
      { name: "session", value: input.session, url: input.base },
    ]);
    const page = await context.newPage();
    page.setDefaultTimeout(12000);
    const errors = [];
    page.on("pageerror", (e) => {
      errors.push(e.message);
      console.error("Browser error:", e.message);
    });
    const go = async (path) => {
      await page.goto(input.base + path);
      await page.waitForFunction(
        () => !!document.querySelector("#client-state"),
      );
    };
    for (const width of [320, 390, 768, 1440]) {
      await page.setViewportSize({ width, height: 840 });
      for (const path of [
        "/notes",
        "/docs",
        "/contacts",
        "/events",
        "/files",
        "/tasks",
        "/services",
        "/work",
        "/markets",
        "/news",
        "/video",
        "/blog",
        "/services/docs",
        "/admin/config",
      ]) {
        await go(path);
        await page.waitForTimeout(150);
        assert(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth + 1,
          ),
          `${path} overflow at ${width}`,
        );
      }
    }
    await go("/services");
    await page.getByRole("navigation", {name:"Services", exact:true}).locator('a[href="/service/docs"]').click();
    assert.equal(new URL(page.url()).pathname, "/service/docs");
    const listMethod = page.locator("section#docs_list");
    await listMethod.getByRole("heading", {name:"HTTP API", exact:true}).waitFor();
    await listMethod.getByRole("heading", {name:"App SDK", exact:true}).waitFor();
    await listMethod.getByRole("heading", {name:"MCP", exact:true}).waitFor();
    assert.equal(await listMethod.locator("details").count(), 0, "reference still hides controls in disclosures");
    const call = page.waitForResponse((r) =>
      r.url().endsWith("/services/call/docs/list"),
    );
    await listMethod.getByRole("button", { name: "Run", exact: true }).click();
    const response = await call;
    assert.equal(response.status(), 200, "service playground read failed");
    const payload = await response.json();
    await page.waitForFunction(expected => {
      const result = Array.from(document.querySelectorAll("section#docs_list pre")).at(-1);
      try { return JSON.stringify(JSON.parse(result.textContent)) === expected; } catch { return false; }
    }, JSON.stringify(payload));
    await go("/docs?new=1");
    await page
      .getByRole("textbox", { name: "Title", exact: true })
      .fill("Migration test document");
    await page
      .getByRole("textbox", { name: "Body", exact: true })
      .fill("## Preserved content\n\nA saved paragraph.");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await page
      .getByRole("heading", { name: "Preserved content", exact: true })
      .waitFor();
    await page.getByRole("link", { name: "Edit", exact: true }).click();
    await page
      .getByRole("textbox", { name: "Body", exact: true })
      .fill("## Updated content\n\nEdits persist.");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await page
      .getByRole("heading", { name: "Updated content", exact: true })
      .waitFor();
    await go("/notes?new=1");
    await page
      .getByRole("textbox", { name: "Title", exact: true })
      .fill("Migration note");
    await page
      .getByRole("textbox", { name: "Body", exact: true })
      .fill("Keep this note");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await page
      .getByRole("link", { name: "Migration note", exact: true })
      .click();
    await page
      .getByRole("textbox", { name: "Body", exact: true })
      .fill("Keep this edited note");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await page.waitForURL(
      (url) => url.pathname === "/notes" && url.search === "",
    );
    await page.getByText("Keep this edited note", { exact: true }).waitFor();
    await go("/contacts");
    await page.getByRole("button", { name: "New", exact: true }).click();
    await page
      .getByRole("textbox", { name: "Name", exact: true })
      .fill("Test Contact");
    await page
      .getByRole("textbox", { name: "Email", exact: true })
      .fill("fixture@example.test");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await page
      .getByRole("heading", { name: "Test Contact", exact: true })
      .waitFor();
    await go("/tasks");
    await page.getByRole("button", { name: "New", exact: true }).click();
    await page
      .getByRole("textbox", { name: "What needs doing?", exact: true })
      .fill("Migration task");
    await page.getByRole("button", { name: "Add", exact: true }).click();
    await page
      .getByRole("link", { name: "Migration task", exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Done", exact: true })
      .last()
      .click();
    await page.locator("span").getByText("done", { exact: true }).waitFor();
    for (const width of [390, 1440]) {
      await page.setViewportSize({ width, height: 840 });
      for (const route of ["/markets", "/news", "/video", "/blog"]) {
        await go(route);
        await page.locator("main h1").waitFor();
        if (route === "/markets")
          await page
            .getByRole("link", { name: "Chart", exact: true })
            .waitFor();
        if (route === "/blog")
          assert(
            (await page.locator("main").innerText()).length < 1500,
            "blog lists whole posts instead of excerpts",
          );
        await page.screenshot({
          path: `/tmp/mu-repair-${route.slice(1)}-${width}.png`,
          fullPage: true,
        });
      }
    }
    await go("/");
    await page
      .getByRole("navigation", { name: "Apps", exact: true })
      .getByRole("link", { name: "Assistant", exact: true })
      .waitFor();
    assert.equal(
      await page
        .getByRole("textbox", { name: "Message Micro", exact: true })
        .count(),
      0,
      "Home still shows a composer",
    );
    const launcher = page.getByRole("navigation", {name: "Apps", exact: true});
    assert.equal(await launcher.locator('a[href^="/apps/"]').count(), 0, "Home contains user apps");
    assert.equal(await launcher.locator('a[href="/inbox"], a[href="/work"]').count(), 0);
    assert.equal(await launcher.locator("section").count(), 4, "Home categories missing");
    const collapse = page.getByRole("button", {name:"Collapse sidebar", exact:true});
    await collapse.click();
    await page.getByRole("button", {name:"Expand sidebar", exact:true}).waitFor();
    assert((await page.locator("main").boundingBox()).width > 1000, "content still constrained to 4xl");
    await page.reload();
    await page.getByRole("button", {name:"Expand sidebar", exact:true}).click();
    await collapse.waitFor();
    assert.equal(await collapse.evaluate(e => getComputedStyle(e).cursor), "pointer");
    await page
      .getByRole("searchbox", { name: "Find an app", exact: true })
      .fill("Markets");
    assert.equal(
      await page
        .getByRole("navigation", { name: "Apps", exact: true })
        .getByRole("link")
        .count(),
      1,
    );
    await go("/work");
    await page.getByRole("heading", { name: "Work", exact: true }).waitFor();
    await page
      .getByRole("button", { name: "Build with Micro", exact: true })
      .waitFor();
    let generated = false,
      edited = false;
    const builtApp = {
      slug: "test-builder",
      name: "Habit tracker",
      html: "<h1>Habit tracker</h1>",
      public: false,
    };
    await page.route("**/apps/generate", async (route) => {
      assert.equal(
        new URLSearchParams(route.request().postData()).get("description"),
        "Build a habit tracker",
      );
      generated = true;
      await route.fulfill({ json: builtApp });
    });
    await page.route("**/apps/test-builder", (route) =>
      route.fulfill({ contentType: "text/html", body: builtApp.html }),
    );
    await page.route("**/apps/test-builder/ai-edit", async (route) => {
      assert.equal(
        new URLSearchParams(route.request().postData()).get("instruction"),
        "Add a weekly view",
      );
      edited = true;
      await route.fulfill({ json: builtApp });
    });
    await page
      .getByRole("textbox", { name: "What would you like to build?" })
      .fill("Build a habit tracker");
    await page
      .getByRole("button", { name: "Build with Micro", exact: true })
      .click();
    await page
      .frameLocator('iframe[title="Habit tracker preview"]')
      .getByRole("heading", { name: "Habit tracker" })
      .waitFor();
    await page
      .getByRole("textbox", { name: "What should Micro change?" })
      .fill("Add a weekly view");
    await page
      .getByRole("button", { name: "Apply changes", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Apply changes", exact: true })
      .waitFor();
    assert(generated && edited, "builder did not generate and iterate");
    await page.screenshot({ path: "/tmp/mu-repair-work.png", fullPage: true });
    await page.getByRole("tab", { name: "Tasks", exact: true }).click();
    await page.getByRole("button", { name: "New task", exact: true }).click();
    await page
      .getByRole("textbox", { name: "What needs doing?", exact: true })
      .fill("Workspace task");
    await page.getByRole("button", { name: "Add", exact: true }).click();
    await page
      .getByRole("link", { name: "Workspace task", exact: true })
      .waitFor();
    assert.equal(
      await page.locator('nav[aria-label="Main"] a[href="/work"]').count(),
      1,
    );
    await go("/files?new=1");
    await page
      .getByRole("textbox", { name: "Filename", exact: true })
      .fill("migration.txt");
    await page
      .getByRole("textbox", { name: "Contents", exact: true })
      .fill("Stored text");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await page
      .getByRole("link", { name: "migration.txt", exact: true })
      .waitFor();
    await page.getByRole("link", { name: "Edit text", exact: true }).click();
    await page
      .getByRole("textbox", { name: "Contents", exact: true })
      .fill("Revised stored text");
    await page.getByRole("button", { name: "Save file", exact: true }).click();
    await page
      .getByRole("link", { name: "migration.txt", exact: true })
      .waitFor();
    await go("/events?new=1");
    await page
      .getByRole("textbox", { name: "Title", exact: true })
      .fill("Migration event");
    await page.getByLabel("When", { exact: true }).fill("2027-01-20T14:30");
    await page
      .getByRole("button", { name: "Save", exact: true })
      .last()
      .click();
    await page
      .getByRole("heading", { name: "Migration event", exact: true })
      .waitFor();
    await go("/mail");
    await page.getByText("Migration mail", { exact: true }).click();
    await page
      .getByRole("heading", { name: "Migration mail", exact: true })
      .waitFor();
    await page
      .frameLocator('iframe[title="Email message"]')
      .getByText("A preserved message body.", { exact: true })
      .waitFor();
    await page.getByRole("button", { name: "Reply", exact: true }).click();
    assert.equal(
      await page.getByRole("textbox", { name: "To", exact: true }).inputValue(),
      "react_sender",
    );
    assert.equal(
      await page
        .getByRole("textbox", { name: "Subject", exact: true })
        .inputValue(),
      "Re: Migration mail",
    );
    await go("/bookmarks");
    await page.getByRole("button", { name: "New", exact: true }).click();
    await page
      .getByRole("textbox", { name: "URL", exact: true })
      .fill("https://example.test/saved");
    await page
      .getByRole("textbox", { name: "Title", exact: true })
      .fill("Migration bookmark");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await page
      .getByRole("heading", { name: "Migration bookmark", exact: true })
      .waitFor();
    await page.getByText("Private note", { exact: true }).click();
    await page
      .getByRole("textbox", { name: "Note", exact: true })
      .fill("Private annotation");
    await page.getByRole("button", { name: "Save note", exact: true }).click();
    await page
      .getByText("Private annotation", { exact: true })
      .first()
      .waitFor();
    page.once("dialog", (d) => d.accept());
    await page.getByRole("button", { name: "Delete", exact: true }).click();
    await page
      .getByRole("heading", { name: "Migration bookmark", exact: true })
      .waitFor({ state: "hidden" });
    await go("/agent/micro");
    await page.getByRole("textbox", { name: "Message Micro" }).waitFor();
    await page.evaluate(
      (id) =>
        history.replaceState(
          { microConversation: { scope: "react_apps:/agent/micro", id } },
          "",
        ),
      input.older,
    );
    await page.reload();
    await page.getByText("Older selected question", { exact: true }).waitFor();
    assert.equal(new URL(page.url()).search, "");
    let sent;
    await page.route("**/agent", (route) => {
      sent = JSON.parse(route.request().postData());
      return route.fulfill({
        contentType: "text/event-stream",
        body:
          "data: " +
          JSON.stringify({
            type: "response",
            text: "Continued the older conversation",
          }) +
          "\n\n",
      });
    });
    await page
      .getByRole("textbox", { name: "Message Micro" })
      .fill("Continue this discussion");
    await page.getByRole("button", { name: "Send", exact: true }).click();
    await page
      .getByText("Continued the older conversation", { exact: true })
      .waitFor();
    assert.equal(sent.context_id, input.older);
    await page.evaluate(() =>
      history.replaceState(
        { microConversation: { scope: "react_apps:/agent/micro", id: "" } },
        "",
      ),
    );
    await page.reload();
    await page.getByRole("button", { name: "Send", exact: true }).waitFor();
    await page.waitForTimeout(200);
    assert.equal(
      await page.getByText("Newer saved question", { exact: true }).count(),
      0,
    );
    assert.equal(
      await page.getByText("Older selected question", { exact: true }).count(),
      0,
    );
    await page.goto(input.base + "/chat");
    await page.getByRole("heading", { name: "Chat", exact: true }).waitFor();
    await page.waitForFunction(
      () =>
        !Array.from(document.querySelectorAll('[role="status"]')).some((e) =>
          /Loading/.test(e.textContent),
        ),
    );
    await page.goto(input.base + "/services/sdk");
    await page.getByRole("heading", { name: "App SDK", exact: true }).waitFor();
    assert((await page.locator("main").innerText()).includes("mu.service"));
    await page.goto(input.base + "/agent/new");
    await page.getByLabel("Instructions", { exact: true }).waitFor();
    assert(
      (await page.getByLabel("Instructions", { exact: true }).boundingBox())
        .width <= 768,
    );
    // Replay and incoming WebSocket messages exercise scrolling without a model call.
    await context.addInitScript(() => {
      window.WebSocket = class {
        constructor() {
          window.testRoomSocket = this;
          setTimeout(() => {
            this.onopen?.();
            for (let i = 0; i < 35; i++)
              this.onmessage?.({
                data: JSON.stringify({
                  username: "reader",
                  content: "Message " + i + "\nSecond line",
                  timestamp: "2026-09-15T12:00:00Z",
                }),
              });
            this.onmessage?.({
              data: JSON.stringify({
                type: "user_list",
                users: ["reader", "reader"],
              }),
            });
          }, 30);
        }
        close() {}
        send() {}
      };
    });
    await page.goto(input.base + "/chat?id=chat_test");
    const log = page.getByRole("log", { name: "Chat messages" });
    await page.getByText("Message 34", { exact: false }).waitFor();
    assert(
      await log.evaluate(
        (e) => e.scrollHeight - e.scrollTop - e.clientHeight < 80,
      ),
      "room did not open at recent messages",
    );
    await log.evaluate((e) => {
      e.scrollTop = 120;
    });
    await page.waitForTimeout(50);
    await page.evaluate(() =>
      window.testRoomSocket.onmessage({
        data: JSON.stringify({
          username: "reader",
          content: "New arriving message",
          timestamp: "2026-09-15T12:01:00Z",
        }),
      }),
    );
    assert(
      Math.abs((await log.evaluate((e) => e.scrollTop)) - 120) < 3,
      "new message moved a reader in history",
    );
    await page.getByLabel("Auto-scroll", { exact: true }).uncheck();
    await page.reload();
    await page.getByText("Message 34", { exact: false }).waitFor();
    assert.equal(
      await page.getByLabel("Auto-scroll", { exact: true }).isChecked(),
      false,
    );
    assert(
      Math.abs((await log.evaluate((e) => e.scrollTop)) - 120) < 3,
      "room lost saved scroll position",
    );
    const prompt = await page
      .getByRole("textbox", { name: "Message", exact: true })
      .boundingBox();
    assert(prompt.y + prompt.height <= 840, "room composer outside viewport");
    const anon = await browser.newContext({ javaScriptEnabled: false });
    const bare = await anon.newPage();
    await bare.goto(input.base);
    await bare.getByRole("textbox", { name: "Message Micro" }).waitFor();
    assert(
      !(await bare.locator("body").innerText()).includes("Loading"),
      "static home loading text",
    );
    for (const path of ["/about", "/privacy"]) {
      await bare.goto(input.base + path);
      assert(
        (await bare.locator("main").innerText()).length > 300,
        path + " missing static content",
      );
      await bare
        .getByRole("link", { name: "Micro", exact: true })
        .first()
        .waitFor();
      assert.equal(
        await bare.getByText("Loading…", { exact: true }).count(),
        0,
      );
    }
    await anon.close();
    await page.setViewportSize({ width: 390, height: 840 });
    await page.waitForTimeout(100);
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
      "mobile room overflows",
    );
    const mobilePrompt = await page
      .getByRole("textbox", { name: "Message", exact: true })
      .boundingBox();
    assert(
      mobilePrompt.y + mobilePrompt.height <= 840,
      "mobile room composer outside viewport",
    );
    assert.deepEqual(errors, []);
    assert(
      await page
        .locator("main h1 img")
        .evaluate((img) => img.complete && img.naturalWidth > 0),
      "native app icon did not load",
    );
    await page.screenshot({
      path: "/tmp/mu-apps-verified.png",
      fullPage: true,
    });
    console.log(
      "Application browser checks passed: mobile widths, document create/edit, notes create/edit, contacts, tasks, file editing, event creation, mail rendering/reply, and no-JS landing.",
    );
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
