/**
 * Single place that knows where the API lives.
 *
 * The frontend addresses media and every other resource by application-level
 * ID through this base URL. It must never build fake-GCS URLs or object paths.
 */
const baseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:18080";

export function apiUrl(path: string): string {
  return `${baseUrl}${path}`;
}
