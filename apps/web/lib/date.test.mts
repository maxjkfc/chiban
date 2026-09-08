import assert from "node:assert/strict";
import test from "node:test";

import { isTodayDate } from "./date.mts";

test("recognizes a date selected by the picker when it is today", () => {
  const now = new Date("2026-08-20T00:30:00.000Z");

  assert.equal(isTodayDate("2026-08-20", "Asia/Taipei", now), true);
});

test("uses the profile timezone at the UTC day boundary", () => {
  const now = new Date("2026-08-19T16:30:00.000Z");

  assert.equal(isTodayDate("2026-08-20", "Asia/Taipei", now), true);
  assert.equal(isTodayDate("2026-08-19", "America/Los_Angeles", now), true);
});

test("rejects a different selected date", () => {
  const now = new Date("2026-08-20T00:30:00.000Z");

  assert.equal(isTodayDate("2026-08-19", "Asia/Taipei", now), false);
});

test("treats the unfiltered date as today", () => {
  assert.equal(isTodayDate(null, "Asia/Taipei", new Date()), true);
});
