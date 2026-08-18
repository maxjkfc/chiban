/**
 * Single place that knows where the API lives and how to talk to it.
 *
 * The frontend addresses media and every other resource by application-level
 * ID through this base URL. It must never build fake-GCS URLs or object paths.
 */
const baseUrl =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:18080";

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
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
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
  /**
   * Application-level ID for the picture, absent when none is set. A new
   * upload gets a new ID, so a cached URL never shows the previous face.
   */
  avatar_media_id?: string;
};

export type GroupMember = {
  user_id: string;
  display_name: string;
  avatar_media_id?: string;
  role: "owner" | "member";
};

export function avatarUrl(mediaId: string): string {
  return apiUrl(`/api/v1/avatars/${mediaId}`);
}

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
  /** Whether this reader may edit it. Sharing means people who cannot now
   * read the same shape. */
  is_owner: boolean;
};

/** "2026-03-15T12:30" to "12:30", for the compact list view. */
export function localTimeOfDay(eatenAtLocal: string): string {
  return eatenAtLocal.slice(11, 16);
}

/** Fetches a meal, or null when it is gone or not visible to this reader. */
export async function fetchMeal(
  mealId: string,
  signal?: AbortSignal,
): Promise<Meal | null> {
  try {
    return await apiFetch<Meal>(`/api/v1/meals/${mealId}`, { signal });
  } catch (caught) {
    // 404 covers both "deleted" and "not shared with you" on purpose, and the
    // card says the same thing either way: there is nothing here to show.
    if (caught instanceof ApiRequestError && caught.status === 404) return null;
    throw caught;
  }
}

export function mealImageUrl(imageId: string): string {
  return apiUrl(`/api/v1/meal-images/${imageId}`);
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

export type ChatMessage = {
  id: string;
  group_id: string;
  user_id: string;
  type: string;
  content: string;
  /** The sender's own id for the message, which makes a retry safe. */
  client_message_id: string;
  created_at: string;
  /** A removed message keeps its place with its content gone, so the replies
   * underneath it still read as answers to something. */
  deleted: boolean;
  /** Set on a meal card. The meal itself is fetched by this id, never carried
   * in the message, so the card is subject to the meal's own authorization. */
  meal_record_id?: string;
  reply_to?: ReplyPreview;
  reactions: MessageReaction[];
};

/** As much of a quoted message as the reply needs to show. */
export type ReplyPreview = {
  id: string;
  user_id: string;
  content: string;
  deleted: boolean;
};

export type MessageReaction = {
  reaction_type: string;
  count: number;
  /** Whether tapping again would take it back. */
  mine: boolean;
};

export type ReactionChange = {
  message_id: string;
  user_id: string;
  reaction_type: string;
  added: boolean;
};

/** One push from the realtime feed. The kind decides which field is filled. */
export type SocketEvent =
  | { type: "message"; message: ChatMessage }
  | { type: "reaction"; reaction: ReactionChange }
  | { type: "deleted"; message_id: string };

export type MessagePage = {
  messages: ChatMessage[];
  /** Cursor for the page before this one; absent once history runs out. */
  before?: string;
};

/**
 * The realtime feed for a group. Same origin and same session cookie as the
 * REST API, so the socket is authorised by the handshake like any other request.
 */
export function chatSocketUrl(groupId: string): string {
  return apiUrl(`/api/v1/ws/groups/${groupId}`).replace(/^http/, "ws");
}

/** "2026-03-15T04:05:06Z" to "12:05" in the reader's own timezone. */
export function timeOfDay(isoInstant: string): string {
  return new Date(isoInstant).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });
}
