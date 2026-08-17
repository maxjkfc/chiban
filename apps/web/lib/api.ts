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
