import assert from "node:assert/strict";
import test from "node:test";

import {
  canShiftTrophyAnchor,
  calendarCells,
  isDailySummaryRangeWithinLimit,
  monthRange,
  shiftMonth,
  trophyAnchorBounds,
  trophyProgress,
  weekRange,
  type DailySummaryDay,
} from "./daily-summary.mts";
import { fetchDailySummary } from "./api.ts";

function day(date: string, mealCount: number): DailySummaryDay {
  return { date, meal_count: mealCount, meal_types: [] };
}

test("maps daily meal counts to the four trophy progress states", () => {
  assert.deepEqual(
    [0, 1, 2, 3, 7].map(trophyProgress),
    [0, 1, 2, 3, 3],
  );
});

test("builds the seven-day range around the profile-local today", () => {
  assert.deepEqual(weekRange("2026-08-20"), {
    start: "2026-08-17",
    end: "2026-08-23",
  });
  assert.deepEqual(weekRange("2026-12-31"), {
    start: "2026-12-28",
    end: "2027-01-03",
  });
});

test("keeps trophy anchors inside the 92-day navigation boundary", () => {
  assert.deepEqual(trophyAnchorBounds("2026-08-20"), {
    start: "2026-05-20",
    end: "2026-11-20",
  });
  assert.equal(canShiftTrophyAnchor("2026-08-20", -1, "2026-08-20"), true);
  assert.equal(canShiftTrophyAnchor("2026-05-20", -1, "2026-08-20"), false);
  assert.equal(canShiftTrophyAnchor("2026-11-20", 1, "2026-08-20"), false);
});

test("never builds a daily-summary request larger than the backend limit", () => {
  assert.equal(
    isDailySummaryRangeWithinLimit({ start: "2026-01-01", end: "2026-04-02" }),
    true,
  );
  assert.equal(
    isDailySummaryRangeWithinLimit({ start: "2026-01-01", end: "2026-04-03" }),
    false,
  );
  assert.equal(
    isDailySummaryRangeWithinLimit({ start: "2026-04-02", end: "2026-01-01" }),
    false,
  );
});

test("month navigation crosses year boundaries and keeps an inclusive range", () => {
  assert.equal(shiftMonth("2026-01", -1), "2025-12");
  assert.equal(shiftMonth("2026-12", 1), "2027-01");
  assert.deepEqual(monthRange("2026-02"), {
    start: "2026-02-01",
    end: "2026-02-28",
  });
});

test("creates a Monday-first calendar grid with every month day", () => {
  const cells = calendarCells("2026-03");
  const dates = cells.filter((value): value is string => value !== null);

  assert.equal(cells.length % 7, 0);
  assert.equal(dates.length, 31);
  assert.equal(dates[0], "2026-03-01");
  assert.equal(dates.at(-1), "2026-03-31");
});

test("fetches both trophy and calendar data through the daily-summary contract", async () => {
  const originalFetch = globalThis.fetch;
  const requests: string[] = [];
  globalThis.fetch = async (input) => {
    requests.push(String(input));
    return new Response(JSON.stringify({ days: [day("2026-08-20", 2)] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  try {
    const week = await fetchDailySummary("2026-08-17", "2026-08-23");
    const month = await fetchDailySummary("2026-08-01", "2026-08-31");

    assert.equal(week.days[0].meal_count, 2);
    assert.equal(month.days[0].date, "2026-08-20");
    assert.deepEqual(requests, [
      "http://localhost:18080/api/v1/meals/daily-summary?start=2026-08-17&end=2026-08-23",
      "http://localhost:18080/api/v1/meals/daily-summary?start=2026-08-01&end=2026-08-31",
    ]);
  } finally {
    globalThis.fetch = originalFetch;
  }
});
