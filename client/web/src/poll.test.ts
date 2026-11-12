import test from "node:test";
import assert from "node:assert/strict";
import { ReadPoll } from "./poll.ts";

test("pending and failing reads have bounded hourly request rates", () => {
  for (const pages of [1, 10]) {
    for (const failed of [false, true]) {
      const poll = new ReadPoll(() => true);
      let elapsed = 0,
        calls = 0;
      while (elapsed < 3600000) {
        elapsed += poll.next(failed, pages);
        calls += pages;
      }
      assert.ok(calls <= 150, `${calls} requests/hour with ${pages} pages`);
    }
  }
});
test("hidden/offline views retain one read, resume once and cancel on navigation", (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let visible = true,
    calls = 0;
  const poll = new ReadPoll(() => visible);
  poll.schedule(() => calls++, 3000);
  visible = false;
  poll.resume();
  t.mock.timers.tick(3600000);
  assert.equal(calls, 0);
  visible = true;
  poll.resume();
  poll.resume();
  assert.equal(calls, 1);
  poll.schedule(() => calls++, 3000);
  poll.cancel();
  t.mock.timers.tick(3600000);
  poll.resume();
  assert.equal(calls, 1);
});
test("backoff is preserved on resume and resets after completion", () => {
  const poll = new ReadPoll(() => true);
  assert.equal(poll.next(), 3000);
  poll.resume();
  assert.equal(poll.next(), 6000);
  poll.cancel();
  assert.equal(poll.next(), 3000);
});
