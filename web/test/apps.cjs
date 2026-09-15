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
    page.on("pageerror", (e) => errors.push(e.message));
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
    await page.getByText("done", { exact: true }).waitFor();
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
    const anon = await browser.newContext({ javaScriptEnabled: false });
    const bare = await anon.newPage();
    await bare.goto(input.base);
    await bare.getByRole("textbox", { name: "Message Micro" }).waitFor();
    assert(
      !(await bare.locator("body").innerText()).includes("Loading"),
      "static home loading text",
    );
    await anon.close();
    assert.deepEqual(errors, []);
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
