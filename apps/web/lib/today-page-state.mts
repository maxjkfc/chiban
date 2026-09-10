import { isTodayDate } from "./date.mts";

/** Only the latest non-aborted request may update page state. */
export function isCurrentAsyncRequest(
  signal: AbortSignal | undefined,
  requestKey: string,
  activeRequestKey: string | null,
): boolean {
  return signal?.aborted !== true && requestKey === activeRequestKey;
}

/**
 * Resolves the Today page heading and empty-state date condition.
 *
 * Before the profile loads, an unfiltered request is the best available
 * indication of today. Once the profile timezone is known, the selected
 * calendar date must be compared in that timezone instead of relying on the
 * browser's local timezone.
 */
export function isTodaySelection(
  date: string | null,
  timezone: string | null,
  now = new Date(),
): boolean {
  return timezone === null ? date === null : isTodayDate(date, timezone, now);
}