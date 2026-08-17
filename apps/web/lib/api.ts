/**
 * Single place that knows where the API lives and how to talk to it.
 *
 * The frontend addresses media and every other resource by application-level
 * ID through this base URL. It must never build fake-GCS URLs or object paths.
 */
const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:18080";

export function apiUrl(path: string): string {
  return `${baseUrl}${path}`;
}

export type ApiError = {
  error: string;
  field?: string;
};

export class ApiRequestError extends Error {
  constructor(
    readonly status: number,
    readonly field?: string,
    message?: string,
  ) {
    super(message ?? "請求失敗");
  }
}

type RequestOptions = {
  method?: string;
  body?: unknown;
  signal?: AbortSignal;
};

/**
 * `credentials: "include"` on every call: the session lives in an HttpOnly
 * cookie on the API's origin, which the browser only attaches when asked.
 */
export async function apiFetch<T>(
  path: string,
  { method = "GET", body, signal }: RequestOptions = {},
): Promise<T> {
  const response = await fetch(apiUrl(path), {
    method,
    signal,
    credentials: "include",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  if (!response.ok) {
    let detail: ApiError | undefined;
    try {
      detail = (await response.json()) as ApiError;
    } catch {
      // Non-JSON error bodies are not worth special handling.
    }
    throw new ApiRequestError(response.status, detail?.field, detail?.error);
  }

  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export type User = {
  id: string;
  email: string;
};

export type Profile = {
  display_name: string;
  timezone: string;
};

/** The browser's own IANA zone, used to prefill onboarding. */
export function detectTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

export type Group = {
  id: string;
  name: string;
  role: "owner" | "member";
  is_owner: boolean;
};

export type GroupMember = {
  user_id: string;
  display_name: string;
  role: "owner" | "member";
};

export type Invite = {
  id: string;
  code: string;
  expires_at: string;
};

/**
 * Sanitises a `?next=` value into a same-origin path.
 *
 * A prefix check on "/" is not enough: the URL parser normalises backslashes
 * in special schemes, so "/\\evil.example" resolves to a different origin.
 * Resolving against the current origin and comparing is the only check that
 * cannot be talked around.
 */
export function safeNextPath(next: string | null | undefined): string | null {
  if (!next) return null;
  try {
    const resolved = new URL(next, window.location.origin);
    if (resolved.origin !== window.location.origin) return null;
    return resolved.pathname + resolved.search;
  } catch {
    return null;
  }
}

export type Meal = {
  id: string;
  meal_type?: string;
  /** The instant, for machines. */
  eaten_at: string;
  /**
   * The same moment as wall-clock time in the owner's profile timezone, e.g.
   * "2026-03-15T12:30". Display and edit this, never `eaten_at`: the browser's
   * timezone is not necessarily the profile's, and converting here would show
   * the wrong time and rewrite the instant on save.
   */
  eaten_at_local: string;
  description?: string;
  photo_ids: string[];
};

/** "2026-03-15T12:30" to "12:30", for the compact list view. */
export function localTimeOfDay(eatenAtLocal: string): string {
  return eatenAtLocal.slice(11, 16);
}

export type MealDay = {
  date: string;
  meals: Meal[];
};

const mealTypeLabels: Record<string, string> = {
  breakfast: "早餐",
  lunch: "午餐",
  dinner: "晚餐",
  snack: "點心",
  other: "其他",
};

export function mealTypeLabel(mealType: string | undefined): string | null {
  return mealType ? (mealTypeLabels[mealType] ?? null) : null;
}

/** Shifts a YYYY-MM-DD date by whole days without touching timezones. */
export function shiftDate(date: string, days: number): string {
  const shifted = new Date(`${date}T00:00:00Z`);
  shifted.setUTCDate(shifted.getUTCDate() + days);
  return shifted.toISOString().slice(0, 10);
}
