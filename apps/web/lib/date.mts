/** Returns a calendar date in an IANA timezone without using the host timezone. */
export function dateInTimezone(now: Date, timezone: string): string {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(now);
  const values = Object.fromEntries(
    parts
      .filter(({ type }) => type !== "literal")
      .map(({ type, value }) => [type, value]),
  );

  return `${values.year}-${values.month}-${values.day}`;
}

/** True when the selected calendar date is today in the user's timezone. */
export function isTodayDate(
  date: string | null,
  timezone: string,
  now = new Date(),
): boolean {
  return date === null || date === dateInTimezone(now, timezone);
}
