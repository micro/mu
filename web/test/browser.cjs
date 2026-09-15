const fs = require("fs"),
  assert = require("assert"),
  path = require("path");
const { chromium } = require(process.env.MU_PLAYWRIGHT_MODULE || "playwright");
(async () => {
  const input = JSON.parse(fs.readFileSync(0, "utf8"));
  const browser = await chromium.launch({
    executablePath: process.env.MU_LAYOUT_BROWSER,
    headless: true,
    args: ["--no-sandbox", "--disable-dev-shm-usage"],
  });
  const errors = [],
    failures = [];
  try {
    const context = await browser.newContext();
    const page = await context.newPage();
    page.setDefaultTimeout(10000);
    page.on("pageerror", (e) => errors.push(e.message));
    await context.addInitScript(() => {
      window.SpeechRecognition = class {
        start() {
          this.onstart?.();
        }
        stop() {
          this.onend?.();
        }
        abort() {}
      };
    });
    await page.route("**/agent", (route) => {
      const body = JSON.parse(route.request().postData());
      const text =
        "## Fruit names\n\n**Arabic** words\n\n" +
        (body.prompt.includes("long")
          ? "A longer paragraph to make the answer scroll.\n\n".repeat(45)
          : "");
      route.fulfill({
        contentType: "text/event-stream",
        body:
          "data: " +
          JSON.stringify({ type: "stream_token", text: "# Raw markdown" }) +
          "\n\ndata: " +
          JSON.stringify({
            type: "response",
            text,
            results: [
              {
                kind: "article",
                title: "Fruit names",
                url: "https://example.com/fruit",
                summary: "A useful article to keep.",
              },
            ],
          }) +
          "\n\n",
      });
    });
    const checkWidth = async () =>
      assert(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth + 1,
        ),
        "document has horizontal overflow",
      );
    for (const width of [320, 390, 768, 1440]) {
      await context.clearCookies();
      await page.setViewportSize({ width, height: 840 });
      await page.goto(input.base + "/");
      await page.getByRole("textbox", { name: "Message Micro" }).waitFor();
      assert.equal(await page.title(), "Home | Micro");
      assert.equal(await page.locator("footer").count(), 1);
      const prompt = page
        .getByRole("textbox", { name: "Message Micro" })
        .locator("..");
      const before = await prompt.boundingBox();
      await page.getByRole("button", { name: "Dictate", exact: true }).click();
      const after = await prompt.boundingBox();
      assert(Math.abs(before.y - after.y) < 2, "dictation moved prompt");
      await page.getByRole("button", { name: "Stop dictation" }).click();
      await page
        .getByRole("textbox", { name: "Message Micro" })
        .fill("Give me a long answer");
      await page.getByRole("button", { name: "Send", exact: true }).click();
      await page
        .getByRole("heading", { name: "Fruit names", exact: true })
        .first()
        .waitFor();
      assert(
        !(await page.getByText("# Raw markdown", { exact: true }).count()),
        "raw markdown is visible",
      );
      assert(
        await page
          .getByRole("log")
          .evaluate((e) => getComputedStyle(e).scrollbarWidth === "none"),
        "visible chat scrollbar",
      );
      assert(
        await page.getByRole("log").evaluate((e) => {
          const q = e.querySelector("[data-question]");
          return (
            Math.abs(
              q.getBoundingClientRect().top -
                e.getBoundingClientRect().top -
                16,
            ) < 3
          );
        }),
        "question is not anchored at start of answer",
      );
      const atBottom = await prompt.boundingBox();
      assert(
        atBottom.y > 600 && atBottom.y + atBottom.height < 840,
        "prompt is not near bottom after response",
      );
      await checkWidth();
      // Guest state in this context is discarded before testing the signed-in flow.
      await page.evaluate(() => sessionStorage.clear());
      await context.addCookies([
        { name: "session", value: input.session, url: input.base },
      ]);
      for (const route of [
        "/assistant",
        "/inbox",
        "/inbox?view=requests",
        "/inbox?view=history&page=2",
        "/inbox?id=" + input.thread,
        "/inbox/new",
        "/inbox/settings",
        "/admin/users",
        "/account",
        "/account/profile",
        "/account/billing",
        "/apps",
      ]) {
        try {
          await page.goto(input.base + route);
          await page.locator("main").waitFor();
          await page.waitForFunction(
            () =>
              !Array.from(document.querySelectorAll("[role=status]")).some(
                (e) => e.textContent.startsWith("Loading"),
              ),
          );
          await checkWidth();
          assert.equal(
            await page.locator("footer").count(),
            0,
            "landing footer in account app",
          );
          assert.equal(
            await page.locator("#mobile-nav").count(),
            0,
            "bottom bar duplicates sidebar",
          );
          if (route === "/assistant") {
            await page
              .getByText("My saved question", { exact: true })
              .waitFor();
            const box = await page
              .getByRole("textbox", { name: "Message Micro" })
              .boundingBox();
            assert(
              box.y > 680 && box.y + box.height < 840,
              "signed-in prompt misplaced",
            );
            assert.equal(
              new URL(page.url()).search,
              "",
              "session identifier in address",
            );
          }
          if (route === "/inbox") {
            const rows = page.locator('main a[href^="/inbox?id="]');
            assert.equal(await rows.count(), 25);
            assert(
              (await rows.first().boundingBox()).height < 116,
              "inbox rows too tall",
            );
            await rows.first().hover();
            assert(
              !(await rows
                .first()
                .evaluate((e) =>
                  getComputedStyle(e).textDecorationLine.includes("underline"),
                )),
              "row hover underlines text",
            );
          }
          if (route.includes("page=2"))
            assert(
              (await page.locator('main a[href^="/inbox?id="]').count()) > 0,
              "older page is empty",
            );
          if (route.includes("?id=")) {
            await page
              .getByRole("button", { name: "Reply", exact: true })
              .click();
            await page
              .getByRole("textbox", { name: "Reply", exact: true })
              .fill("Keep this draft");
            await page
              .getByRole("button", { name: "Send", exact: true })
              .click();
            await page.getByRole("alert").waitFor();
            assert.equal(
              await page
                .getByRole("textbox", { name: "Reply", exact: true })
                .inputValue(),
              "Keep this draft",
              "failed reply lost draft",
            );
            const previous = await page.locator("article").count();
            await page
              .getByRole("button", { name: "Earlier messages" })
              .click();
            await page.waitForFunction(
              (n) => document.querySelectorAll("article").length > n,
              previous,
            );
            assert.equal(
              await page
                .locator('iframe[title="Email message"]')
                .getAttribute("sandbox"),
              "allow-same-origin allow-popups allow-popups-to-escape-sandbox",
            );
          }
          if (route === "/inbox/settings") {
            const morning = page.getByRole("switch", {
                name: "Morning brief",
                exact: true,
              }),
              news = page.getByRole("switch", {
                name: "Include world news",
                exact: true,
              });
            if ((await news.getAttribute("aria-checked")) === "true") {
              await news.click();
              await page
                .getByRole("status")
                .filter({ hasText: "Saved." })
                .waitFor();
            }
            await morning.click();
            await page
              .getByRole("status")
              .filter({ hasText: "Saved." })
              .waitFor();
            await page.reload();
            await morning.waitFor();
            assert.equal(await morning.getAttribute("aria-checked"), "false");
            assert.equal(await news.getAttribute("aria-checked"), "false");
            await morning.click();
            await page
              .getByRole("status")
              .filter({ hasText: "Saved." })
              .waitFor();
            assert.equal(await news.getAttribute("aria-checked"), "false");
            assert.equal(
              await page.locator('a[href^="/events"]').count(),
              0,
              "settings links to calendar",
            );
          }
          if (route === "/admin/users") {
            assert.equal(
              await page.getByRole("button", { name: /Actions for/ }).count(),
              25,
            );
            await page
              .getByRole("button", { name: "Next", exact: true })
              .click();
            await page.getByText(/^2 of /).waitFor();
            await page
              .getByRole("button", { name: /Actions for/ })
              .first()
              .click();
            await page.getByRole("menu").waitFor();
            const menu = await page.getByRole("menu").boundingBox();
            assert(
              menu.x >= 0 && menu.x + menu.width <= width,
              "action menu outside mobile viewport",
            );
            await page.keyboard.press("Escape");
          }
          if (route === "/account") {
            await page
              .getByRole("heading", { name: "Connections", exact: true })
              .waitFor();
            assert.equal(
              await page
                .getByRole("link", { name: "Mail settings", exact: true })
                .count(),
              1,
            );
          }
          if (route === "/account/billing") {
            assert.equal(
              await page
                .getByRole("link", { name: "Transfer", exact: true })
                .getAttribute("href"),
              "/account/transfer",
            );
            await page.getByText("1 credit = 1¢", { exact: true }).waitFor();
            await page
              .getByText(
                "Your own calls are not charged because you are an admin.",
              )
              .waitFor();
          }
          if (route === "/account/profile") {
            await page
              .getByLabel("Display name", { exact: true })
              .fill("Alex Updated");
            await page
              .getByRole("button", { name: "Save", exact: true })
              .first()
              .click();
            await page
              .getByRole("status")
              .filter({ hasText: "Saved." })
              .waitFor();
            await page.reload();
            assert.equal(
              await page
                .getByLabel("Display name", { exact: true })
                .inputValue(),
              "Alex Updated",
            );
          }
          if (route === "/apps") {
            await page
              .getByRole("heading", { name: "Fruit cards", exact: true })
              .waitFor();
            await page
              .getByRole("searchbox", { name: "Find apps" })
              .fill("Fruit");
            await page
              .getByRole("heading", { name: "Fruit cards", exact: true })
              .waitFor();
          }
          if (width < 768) {
            await page.getByRole("button", { name: "Open navigation" }).click();
            await page.getByRole("dialog").waitFor();
            assert.equal(
              await page
                .getByRole("dialog")
                .getByRole("navigation", { name: "Main", exact: true })
                .getByRole("link")
                .count(),
              5,
            );
            await page.keyboard.press("Escape");
            assert.equal(await page.getByRole("dialog").count(), 0);
          }
          if (process.env.MU_LAYOUT_SHOTS) {
            fs.mkdirSync(process.env.MU_LAYOUT_SHOTS, { recursive: true });
            await page.screenshot({
              path: path.join(
                process.env.MU_LAYOUT_SHOTS,
                "react-" +
                  width +
                  "-" +
                  route.replace(/[^a-z0-9]/gi, "_") +
                  ".png",
              ),
              fullPage: true,
            });
          }
          console.log("PASS", width, route);
        } catch (e) {
          failures.push(width + " " + route + ": " + e.message);
        }
      }
    }
    await page.setViewportSize({ width: 390, height: 840 });
    await page.goto(input.base + "/inbox");
    await page
      .getByRole("link", { name: "Message requests (2)", exact: true })
      .click();
    await page.getByText("Waiting SMS", { exact: true }).click();
    await page
      .getByRole("region", { name: "Message request", exact: true })
      .waitFor();
    assert.equal(
      await page.getByLabel("Ask Micro about this").count(),
      0,
      "held message offers agent actions before approval",
    );
    await page.getByRole("button", { name: "Let in", exact: true }).click();
    await page.waitForURL(input.base + "/inbox");
    await page
      .getByRole("link", { name: "Message requests (1)", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Block sender", exact: true })
      .click();
    await page.getByRole("button", { name: "Blocked", exact: true }).waitFor();
    assert(
      await page
        .getByRole("button", { name: "Blocked", exact: true })
        .isDisabled(),
    );
    await checkWidth();
    await page.goto(input.base + "/inbox?id=" + input.thread);
    await page.getByRole("button", { name: "More actions" }).click();
    await page
      .getByRole("menuitem", { name: "Mark unread", exact: true })
      .click();
    await page.waitForURL(input.base + "/inbox");
    const list = await context.request.get(input.base + "/inbox", {
      headers: { Accept: "application/json" },
    });
    assert(
      (await list.json()).items.find((row) => row.id === input.thread)?.unread,
      "mark unread was immediately undone",
    );
    const csrf = (await context.cookies()).find(
      (c) => c.name === "csrf_token",
    ).value;
    for (const headers of [
      { Accept: "application/json" },
      { Accept: "application/json", "X-CSRF-Token": csrf },
    ]) {
      const forbidden = await context.request.post(input.base + "/inbox/held", {
        headers,
        form: { id: input.other, do: "let" },
      });
      assert.equal(forbidden.status(), headers["X-CSRF-Token"] ? 404 : 403);
    }
    const response = await context.request.get(
      input.base + "/inbox?id=" + input.other,
      { headers: { Accept: "application/json" } },
    );
    assert.equal(response.status(), 404, "another owner’s conversation leaked");
    // Model a standalone viewport: safe areas and an overlay keyboard which
    // resizes visualViewport without changing the layout viewport.
    await page.setViewportSize({ width: 390, height: 840 });
    for (const route of ["/assistant", "/inbox", "/account/profile", "/apps"]) {
      await page.goto(input.base + route);
      await page.locator(".app-shell").waitFor();
      await page.evaluate(() => {
        const shell = document.querySelector(".app-shell");
        shell.style.setProperty("--safe-top", "44px");
        shell.style.setProperty("--safe-bottom", "34px");
        shell.style.setProperty("--safe-left", "12px");
        shell.style.setProperty("--safe-right", "12px");
      });
      const header = await page.locator("header").boundingBox();
      assert(
        header.y >= 44 && header.x >= 12,
        "page overlaps installed-app safe area: " + route,
      );
      await checkWidth();
      if (route !== "/assistant") continue;
      const composer = page
        .getByRole("textbox", { name: "Message Micro" })
        .locator("..");
      await composer.waitFor();
      for (const [height, offsetTop] of [
        [840, 0],
        [460, 0],
        [420, 40],
        [840, 0],
      ]) {
        await page.evaluate(
          ({ height, offsetTop }) => {
            Object.defineProperty(visualViewport, "height", {
              configurable: true,
              value: height,
            });
            Object.defineProperty(visualViewport, "offsetTop", {
              configurable: true,
              value: offsetTop,
            });
            visualViewport.dispatchEvent(new Event("resize"));
            visualViewport.dispatchEvent(new Event("scroll"));
          },
          { height, offsetTop },
        );
        await page.waitForFunction(
          ({ bottom }) => {
            const form = document.querySelector(
              'textarea[aria-label="Message Micro"]',
            ).form;
            const rect = form.getBoundingClientRect();
            return rect.bottom <= bottom - 34 && rect.bottom >= bottom - 70;
          },
          { bottom: height + offsetTop },
        );
        const box = await composer.boundingBox();
        assert(box.y >= 100, "keyboard pushes composer over header");
      }
    }
    for (const route of [
      "/about",
      "/privacy",
      "/contact",
      "/pricing",
      "/status",
    ]) {
      await page.goto(input.base + route);
      await page.locator("main h1").waitFor();
      await page.waitForFunction(
        () =>
          !Array.from(document.querySelectorAll('[role="status"]')).some((e) =>
            /Loading/.test(e.textContent),
          ),
      );
      assert.equal(await page.locator("footer a").count(), 5);
      await checkWidth();
      const api = await context.request.get(input.base + route, {
        headers: { Accept: "application/json" },
      });
      if (!["/about", "/privacy"].includes(route))
        assert(api.headers()["cache-control"].includes("no-store"));
    }
    // Paid pricing must render the actual JSON contract, not Go field names.
    const paidPricing = {
      payments: true,
      topup: true,
      question_cost: 2,
      welcome: 100,
      daily: 10,
      prices: [
        {
          operation: "app_build",
          description: "Build an app",
          cost: 25,
          unit: "credits",
        },
        {
          operation: "news_search",
          description: "Search news",
          cost: 0,
          unit: "credits",
        },
      ],
      limits: [{ label: "Mail", limit: 50 }],
    };
    await page.route("**/pricing", async (route) => {
      if (route.request().headers().accept?.includes("application/json"))
        return route.fulfill({ json: paidPricing });
      const response = await route.fetch();
      const html = (await response.text()).replace(
        /<script id="client-data" type="application\/json">[\s\S]*?<\/script>/,
        '<script id="client-data" type="application/json">' +
          JSON.stringify({ page: paidPricing }) +
          "</script>",
      );
      await route.fulfill({ response, body: html });
    });
    for (const width of [320, 1440]) {
      await page.setViewportSize({ width, height: 840 });
      await page.goto(input.base + "/pricing");
      const row = page.getByRole("row").filter({ hasText: "Build an app" });
      await row.waitFor();
      assert.equal(await row.getByRole("cell").innerText(), "25");
      assert.equal(
        await page
          .getByRole("row")
          .filter({ hasText: "Search news" })
          .getByRole("cell")
          .innerText(),
        "Free",
      );
      await checkWidth();
    }
    await page.unroute("**/pricing");
    // A JSON read of a page URL must not poison browser back navigation.
    await page.goto(input.base + "/inbox");
    await page.evaluate(() =>
      fetch("/inbox", { headers: { Accept: "application/json" } }).then((r) =>
        r.json(),
      ),
    );
    await page.goto(input.base + "/account");
    await page.goBack();
    await page.getByRole("heading", { name: "Inbox", exact: true }).waitFor();
    assert.equal(
      await page.locator("script#client-data").count(),
      1,
      "page data missing from HTML response",
    );
    assert.equal(errors.length, 0, errors.join("\n"));
    assert.equal(failures.length, 0, failures.join("\n"));
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
