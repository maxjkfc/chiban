import assert from "node:assert/strict";
import test from "node:test";

import {
  isCurrentAsyncRequest,
  isTodaySelection,
} from "./today-page-state.mts";

test("marks today after returning from another selected date", () => {
  const now = new Date("2026-08-20T00:30:00.000Z");

  assert.equal(isTodaySelection("2026-08-19", "Asia/Taipei", now), false);
  assert.equal(isTodaySelection("2026-08-20", "Asia/Taipei", now), true);
  assert.equal(isTodaySelection(null, "Asia/Taipei", now), true);
});

test("rejects aborted and stale async responses", () => {
  const active = new AbortController();
  assert.equal(isCurrentAsyncRequest(active.signal, "2026-08-20", "2026-08-20"), true);
  assert.equal(isCurrentAsyncRequest(active.signal, "2026-08-19", "2026-08-20"), false);

  active.abort();
  assert.equal(isCurrentAsyncRequest(active.signal, "2026-08-20", "2026-08-20"), false);
});