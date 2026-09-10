import assert from "node:assert/strict";
import test from "node:test";

import {
  calendarCells,
  monthRange,
  shiftMonth,
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
