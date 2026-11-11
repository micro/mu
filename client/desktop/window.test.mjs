import test from "node:test";
import assert from "node:assert/strict";
import { restoreWindow, menuTemplate } from "./window.mjs";

const primary = { x: 0, y: 24, width: 1440, height: 876 };
const secondary = { x: -1920, y: 0, width: 1920, height: 1080 };

test("window placement survives connected and disconnected monitors", () => {
  const saved = { x: -1800, y: 100, width: 1000, height: 800, maximized: true };
  assert.deepEqual(restoreWindow(saved, [primary, secondary]), saved);
  const recovered = restoreWindow(saved, [primary]);
  assert.equal(recovered.x, 0);
  assert.equal(recovered.y, 100);
});

test("window bounds are clamped and corrupt settings fall back", () => {
  const large = restoreWindow({ x: 100, y: 100, width: 5000, height: 5000 }, [
    primary,
  ]);
  assert.deepEqual(large, { ...primary, maximized: false });
  for (const bad of [
    null,
    {},
    { x: NaN, y: 0, width: 500, height: 600 },
    { x: 0, y: 0, width: -1, height: 600 },
  ]) {
    const value = restoreWindow(bad, [primary]);
    assert.equal(value.width, 1000);
    assert.equal(value.height, 800);
    assert.equal(value.maximized, false);
    assert.ok(value.y >= primary.y);
  }
});

test("native menu routes only to known views and exposes manual downloads", () => {
  const visited = [];
  let downloads = 0;
  const menus = menuTemplate(
    "linux",
    (view) => visited.push(view),
    () => downloads++,
  );
  const items = menus.flatMap((m) => m.submenu || []);
  for (const id of [
    "new-chat",
    "settings",
    "home",
    "inbox",
    "agents",
    "services",
    "chats",
    "account",
  ])
    items.find((i) => i.id === id).click();
  assert.deepEqual(visited, [
    "",
    "settings",
    "home",
    "inbox",
    "agents",
    "services",
    "chats",
    "account",
  ]);
  items.find((i) => i.id === "downloads").click();
  assert.equal(downloads, 1);
  assert.equal(
    menuTemplate(
      "darwin",
      () => {},
      () => {},
    )[0].role,
    "appMenu",
  );
});
