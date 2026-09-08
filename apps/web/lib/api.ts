/**
 * Single place that knows where the API lives and how to talk to it.
 *
 * The frontend addresses media and every other resource by application-level
 * ID through this base URL. It must never build fake-GCS URLs or object paths.
 */
const baseUrl =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:18080";

/**
 * Where the browser reaches the API.
 *
 * An empty base means "wherever this page came from": the deployment puts the
 * API and the site on one hostname and splits them by path, so the built image
 * carries no domain in it and works on any of them. Local development sets an
 * absolute base, because there the two really are on different ports.
 */
export function apiUrl(path: string): string {
  return `${baseUrl}${path}`;
}

export type ApiError = {
  error: string;
  field?: string;
};

export class ApiRequestError extends Error {
  readonly status: number;
  readonly field?: string;

  constructor(status: number, field?: string, message?: string) {
    super(message ?? "請求失敗");
    this.status = status;
    this.field = field;
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
  unread_count?: number;
  has_unread?: boolean;
};

export function markGroupRead(
  groupId: string,
  messageId: string,
): Promise<Group> {
  return apiFetch<Group>(`/api/v1/groups/${groupId}/read`, {
    method: "POST",
    body: { message_id: messageId },
  });
}

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
  /** Set on an image or GIF message. The bytes are read through the media
   * endpoint, which decides who may see them. */
  chat_media_id?: string;
  /** Set on a sticker message. Like the two above it is only a reference. */
  sticker_id?: string;
  reply_to?: ReplyPreview;
  reactions: MessageReaction[];
};

/** As much of a quoted message as the reply needs to show. */
export type ReplyPreview = {
  id: string;
  user_id: string;
  /** What the quoted message was. Only text carries content, so this is the
   * whole preview for a picture, a sticker or a meal card. */
  type: string;
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
export function chatMediaUrl(mediaId: string): string {
  return apiUrl(`/api/v1/chat-media/${mediaId}`);
}

export type ChatMedia = {
  id: string;
  media_type: "image" | "gif";
};

/** Uploads one image or GIF and returns its application id. */
export async function uploadChatMedia(file: File): Promise<ChatMedia> {
  const form = new FormData();
  form.append("file", file);

  const response = await fetch(apiUrl("/api/v1/chat-media"), {
    method: "POST",
    credentials: "include",
    body: form,
  });
  if (!response.ok) {
    const detail = (await response.json().catch(() => null)) as ApiError | null;
    throw new ApiRequestError(response.status, detail?.field, detail?.error);
  }
  return (await response.json()) as ChatMedia;
}

export function stickerUrl(stickerId: string): string {
  return apiUrl(`/api/v1/stickers/${stickerId}/media`);
}

export type Sticker = {
  id: string;
  type: "image" | "gif";
  /**
   * Which quick-rail slot this sticker holds, 1 to MAX_STICKER_PINS. Absent
   * when it is not pinned — the API omits the field rather than sending a zero.
   */
  pin_order?: number;
};

/** How many stickers fit on the chat composer's quick rail. Mirrors the API. */
export const MAX_STICKER_PINS = 4;

/**
 * Replaces the caller's quick rail, in order, and returns the whole library
 * as the server now sees it — so the picker that saved does not need a second
 * request to redraw.
 */
export function setStickerPins(stickerIds: string[]): Promise<Sticker[]> {
  return apiFetch<Sticker[]>("/api/v1/me/sticker-pins", {
    method: "PUT",
    body: { sticker_ids: stickerIds },
  });
}

/** Adds one sticker to the caller's own library. */
export async function uploadSticker(file: File): Promise<Sticker> {
  const form = new FormData();
  form.append("file", file);

  const response = await fetch(apiUrl("/api/v1/me/stickers"), {
    method: "POST",
    credentials: "include",
    body: form,
  });
  if (!response.ok) {
    const detail = (await response.json().catch(() => null)) as ApiError | null;
    throw new ApiRequestError(response.status, detail?.field, detail?.error);
  }
  return (await response.json()) as Sticker;
}

export function chatSocketUrl(groupId: string): string {
  const path = `/api/v1/ws/groups/${groupId}`;
  if (baseUrl) return `${baseUrl}${path}`.replace(/^http/, "ws");

  // Same-origin deployment: a relative path cannot be turned into a socket URL
  // by string substitution, so it is built from the page's own address. That
  // also picks wss automatically on an https page, which is the only thing
  // that works behind the tunnel.
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${window.location.host}${path}`;
}

/** "2026-03-15T04:05:06Z" to "12:05" in the reader's own timezone. */
export function timeOfDay(isoInstant: string): string {
  return new Date(isoInstant).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * "2026-03-15T04:05:06Z" to a day-divider label in the reader's own
 * timezone: "今天" / "昨天", "3月15日" within the current year, otherwise
 * "2025年3月15日".
 */
export function dayLabel(isoInstant: string): string {
  const date = new Date(isoInstant);
  const today = new Date();
  const dateMidnight = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate(),
  ).getTime();
  const todayMidnight = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate(),
  ).getTime();
  const diffDays = Math.round((todayMidnight - dateMidnight) / 86_400_000);

  if (diffDays === 0) return "今天";
  if (diffDays === 1) return "昨天";
  return date.toLocaleDateString("zh-TW", {
    year: date.getFullYear() === today.getFullYear() ? undefined : "numeric",
    month: "long",
    day: "numeric",
  });
}

/** True when two instants fall on different calendar days in the reader's own timezone. */
export function isDifferentDay(a: string, b: string): boolean {
  const da = new Date(a);
  const db = new Date(b);
  return (
    da.getFullYear() !== db.getFullYear() ||
    da.getMonth() !== db.getMonth() ||
    da.getDate() !== db.getDate()
  );
}
