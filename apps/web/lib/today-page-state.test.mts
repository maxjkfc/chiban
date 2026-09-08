import assert from "node:assert/strict";
import test from "node:test";

import { isTodaySelection } from "./today-page-state.mts";

test("marks today after returning from another selected date", () => {
  const now = new Date("2026-08-20T00:30:00.000Z");

  assert.equal(isTodaySelection("2026-08-19", "Asia/Taipei", now), false);
  assert.equal(isTodaySelection("2026-08-20", "Asia/Taipei", now), true);
  assert.equal(isTodaySelection(null, "Asia/Taipei", now), true);
});