import { test } from "node:test";
import assert from "node:assert/strict";
import { paginate, collectAll, type Page } from "./pagination.js";

test("paginate walks every page in order", async () => {
  const pages: Record<string, Page<number>> = {
    "": { items: [1, 2], nextCursor: "p2" },
    p2: { items: [3, 4], nextCursor: "p3" },
    p3: { items: [5], nextCursor: undefined },
  };
  const fetch = async (cursor?: string) => pages[cursor ?? ""];

  const got: number[] = [];
  for await (const item of paginate(fetch)) {
    got.push(item);
  }
  assert.deepEqual(got, [1, 2, 3, 4, 5]);
});

test("collectAll stops at an empty cursor", async () => {
  let calls = 0;
  const fetch = async () => {
    calls++;
    return { items: ["only"], nextCursor: undefined } as Page<string>;
  };
  const items = await collectAll(fetch);
  assert.deepEqual(items, ["only"]);
  assert.equal(calls, 1);
});
