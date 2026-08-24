# Specification: Dual-Track Message & Meal Notification System (In-App Toast + PWA Web Push)

## Problem Statement

Users of Chiban access the application primarily via mobile web browsers (Safari on iOS, Chrome on Android). When a group member shares a meal or sends a chat message:
1. If the user is active on another page within the app (such as the "Today" dashboard or "Profile"), they have no immediate visual indicator that a new message or meal has arrived in their other groups without manually navigating to the group chat.
2. If the user has switched apps, locked their screen, or closed their browser, they receive no system-level notifications, breaking the social accountability loop that encourages real-time interaction and recurring meal logging.

## Solution

A dual-track notification system supporting both foreground in-app awareness and background system push notifications:
1. **Track A (Foreground / In-App Realtime Toast Banner)**: When the user is active inside the web application, a global WebSocket connection (`GET /api/v1/ws/me`) delivers incoming events across all joined groups. If the event belongs to a group not currently open/focused, an in-app top toast banner slides down displaying the sender's avatar, group name, and message/meal summary. Tapping the banner immediately routes the user into that group's chat room.
2. **Track B (Background / PWA Web Push via VAPID & Service Worker)**: For users outside the application or on locked screens, a progressive web app (PWA) setup with Service Worker receives standard Web Push notifications (VAPID). When new activity is committed in PostgreSQL with `inserted == true`, the backend dispatches push payloads to eligible group members who do not hold an active, non-expired focus lease for that specific chat room. Tapping the native system notification wakes/opens the PWA directly to the relevant group chat.

## User Stories

1. As a group member viewing my Today meal summary, I want to see an in-app banner when a friend posts a message in our lunch group, so that I know there is new activity without refreshing or checking the group tab.
2. As a group member on my profile page, I want to see an in-app banner when a friend shares a new meal, so that I can immediately jump in and react to their food.
3. As an active user in Group A, I want to receive an in-app toast notification when someone messages me in Group B, so that I don't miss urgent group discussions while chatting elsewhere.
4. As an active user in Group A, I do not want to receive an in-app toast notification for messages sent within Group A, so that my active conversation screen is not cluttered by duplicate alerts.
5. As an active user reading a chat thread, I want the in-app notification banner to automatically dismiss after a few seconds or allow me to swipe it away, so that it does not permanently block screen content.
6. As a group member tapping on an in-app notification banner, I want to be immediately navigated to the corresponding group chat room, so that I can reply quickly.
7. As a mobile user who added Chiban to my iOS or Android home screen, I want to be prompted to enable notifications with a clear explanation, so that I understand why the permission is needed.
8. As an iOS user running Safari, I want clear instructions on how to "Add to Home Screen" to enable push notifications, so that I am not confused by browser-level platform limitations.
9. As a group member with the app in the background or screen locked, I want to receive a system status bar notification when a meal is shared, so that I am reminded to check and react to my friend's meal.
10. As a group member with the app closed, I want to receive a system status bar notification when someone sends a chat message, image, GIF, or sticker, so that I stay connected with my group.
11. As a group member tapping a system push notification from the lock screen, I want the app to open directly into the target group chat (preserving deep link safely even if re-login is required), so that I can see the context of the notification instantly.
12. As a group member who is currently active and viewing a chat room on this device, I do not want this device to receive a duplicate system lock-screen push notification for messages I am already reading live.
13. As a user logging out of my account on a device, I want my push notification subscription for that device to be unregistered on the server, so that other people using that device do not receive my private group notifications.
14. As a user with multiple devices (e.g. phone and tablet), I want push notifications delivered to all my registered devices that do not hold an active focus lease, so that I can respond from whichever device is idle.
15. As a group member belonging to multiple overlapping groups, I do not want to receive duplicate push notifications when a friend shares a single meal across multiple groups at once.
16. As a system administrator, I want stale or unsubscribed push endpoints (HTTP 404 or 410 from Apple/Google push services) to be pruned automatically from the database, while server-side credential errors (HTTP 401/403) trigger alerts without destroying valid user subscriptions.

## Implementation Decisions

### Scope & Phasing Note
- This specification defines the Phase 9 (Post-V0.1 MVP Extension / V0.1.1) notification enhancement. It builds directly on the V0.1 Chat Core, Meal Sharing, and Auth foundations without altering V0.1 core meal data structures.

### Modules & Architecture

1. **Backend Push Service (`internal/push`)**:
   - Manages VAPID public/private key pairs and subscriber persistence.
   - Provides HTTP endpoints for VAPID public key discovery, subscription registration, and subscription cancellation.
   - Dispatches Web Push payloads asynchronously using standard Web Push protocols (RFC 8030 / RFC 8292) with bounded worker pools (e.g. max 8 concurrent requests) and detached context (`context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)`).
   - Validates client-provided endpoints: Enforces `https://` scheme and restricts hosts to an allowlist (`*.push.apple.com`, `fcm.googleapis.com`, `*.notify.windows.com`, `updates.push.services.mozilla.com`) to prevent SSRF.
   - Strict Push Error Categorization:
     - **Terminal Subscriptions (`404 Not Found`, `410 Gone`)**: Deletes subscription records by subscription row ID.
     - **Server/Credential Errors (`401 Unauthorized`, `403 Forbidden`)**: Logs a critical error and triggers alerts without deleting subscriptions (fail-safe to prevent mass subscriber purging during key rotation or config mistakes).
     - **Transient Errors (`429 Too Many Requests`, `5xx Server Error`)**: Retries with exponential backoff up to 2 times.

2. **Backend Event Orchestration & Realtime Multicast (`internal/chat`, `internal/meal`)**:
   - Maintain the existing architecture: All operations persist to PostgreSQL before notification.
   - Once a message or meal share is saved to PostgreSQL:
     1. Deliver the event to live in-memory WebSocket subscribers via `Hub.Broadcast`.
     2. If and only if `inserted == true` (genuine new record, not an idempotent retry): asynchronously trigger `push.Service.NotifyGroup`, querying eligible group members with active push subscriptions, excluding the sender and endpoints belonging to device connections holding an active, non-expired focus lease for that group.
     3. Cross-Group Meal Share Deduplication: When a single meal is shared into multiple target groups simultaneously, push dispatch groups recipient subscriptions by recipient `user_id` and dispatches at most one push notification per user device.
   - Focus as a Lease (Heartbeat TTL Strategy):
     - Focus is modeled as a short-lived lease (`expires_at = now + 45s`), not persistent state.
     - The client sends `{ "type": "focus", "group_id": "...", "device_id": "..." }` and periodically renews it every 20–30s.
     - The server validates that `user_id` comes strictly from the authenticated session context, indexing focus as `(session_user_id, device_id) -> (group_id, expires_at)`.
     - Server-side Ping / Keep-Alive: `GET /api/v1/ws/me` periodically pings the client (every 30s). If the TCP connection goes half-open or the client stops sending focus heartbeats, the lease expires naturally within 45s and automatically fails open to delivering system push notifications.
   - Dynamic Membership Broadcast:
     - Global WebSocket connection (`GET /api/v1/ws/me`) subscribes at user level. On each broadcast, membership is verified dynamically, ensuring newly joined groups immediately receive toasts and left groups are instantly muted without needing connection resets.

3. **Database Schema (`migrations/00014_push_subscriptions.sql`)**:
   - Table `push_subscriptions`:
     - `id` (UUID, Primary Key DEFAULT gen_random_uuid())
     - `user_id` (UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE)
     - `device_id` (TEXT NOT NULL)
     - `endpoint` (TEXT NOT NULL UNIQUE)
     - `p256dh_key` (TEXT NOT NULL)
     - `auth_key` (TEXT NOT NULL)
     - `user_agent` (TEXT)
     - `created_at` (TIMESTAMPTZ NOT NULL DEFAULT NOW())
     - `updated_at` (TIMESTAMPTZ NOT NULL DEFAULT NOW())
   - Constraints & Indexes:
     - `UNIQUE(endpoint)`
     - `UNIQUE(user_id, device_id)`
     - `INDEX idx_push_subscriptions_user_id ON push_subscriptions(user_id)`
   - Subscription Store Transactional Logic:
     - When registering or rotating endpoints, the store runs a single transaction that deletes any existing row matching `(user_id, device_id)` or `(endpoint)`, and inserts the new record. This guarantees idempotency and prevents double-unique constraint violations on rotation.

4. **API Contracts (`apps/api/internal/httpapi`)**:
   - `GET /api/v1/push/vapid-public-key` -> returns `{ "public_key": "..." }` (Public)
   - `POST /api/v1/push/subscribe` -> accepts `{ "device_id": "...", "endpoint": "...", "old_endpoint": "...", "keys": { "p256dh": "...", "auth": "..." } }` (Requires Auth)
   - `POST /api/v1/push/unsubscribe` -> accepts `{ "endpoint": "..." }` (Requires Auth, deletes row matching `endpoint = $1 AND user_id = $2`)
   - `POST /api/v1/auth/logout` -> accepts optional `{ "device_id": "..." }` in request body; if provided, unregisters the device's push subscription during logout.
   - `GET /api/v1/ws/me` -> Global authenticated WebSocket for in-app events across all user's joined groups with focus lease heartbeat.

5. **Frontend PWA & Service Worker (`apps/web`)**:
   - Durable Device & Subscription Persistence:
     - `device_id` is generated via `crypto.randomUUID()` and stored in `IndexedDB` (accessible by both client windows and Service Worker scopes).
   - Metadata & Manifest (`apps/web/app/layout.tsx` & `public/manifest.json`):
     - `metadata.manifest = "/manifest.json"`
     - `appleWebApp: { capable: true, statusBarStyle: "default", title: "吃伴" }`
     - Icons: 192x192, 512x512 maskable, and 180x180 apple-touch-icon.
     - Caching header for `/sw.js`: `Cache-Control: no-cache, no-store, must-revalidate` to ensure immediate worker updates.
   - iOS WebKit Feature Detection & User Gesture Guarantees:
     - Capability check: Evaluates `'PushManager' in window && 'Notification' in window`. If unsupported (e.g. iOS < 16.4), renders a gentle system requirement notice.
     - Standalone check: If running in iOS Safari tab mode (`!window.matchMedia('(display-mode: standalone)').matches`), renders clear visual instructions: "請先點擊分享按鈕 ➔ 加入主畫面以啟用推播通知".
     - Direct Gesture: `Notification.requestPermission()` is strictly invoked inside direct user tap handlers (e.g. "啟用通知" button), never deferred inside async callbacks.
   - Service Worker (`public/sw.js`):
     - Lifecycle: calls `self.skipWaiting()` on install and `self.clients.claim()` on activate.
     - Push Handler: Deserializes payload `{ title, body, icon, url, group_id, tag }`.
     - Always displays notification within `event.waitUntil(self.registration.showNotification(title, { body, icon: '/icons/icon-192.png', tag: group_id, renotify: true, data: { url } }))` to comply with browser push policies and keep the worker active.
     - Notification Click Handler: Focuses matching open client window or opens target deep link (`url`).
     - Subscription Change Handler (`pushsubscriptionchange`): Retrieves `device_id` from IndexedDB and dispatches resubscribe request to `/api/v1/push/subscribe` with `old_endpoint`.
   - Open Redirect Prevention & Deep Link Preservation:
     - `AuthGate` strictly validates `?next=` query parameter: Must start with a single `/` and must not contain `//` or scheme, preventing open-redirect attacks while preserving legitimate group links.
   - In-App Toast Banner (`components/notification-banner.tsx`):
     - Mounted in `(app)/layout.tsx`.
     - Displays top floating notification card when an event arrives from an unfocused group.
     - Throttling & Merging: Merges multiple messages from the same group into a single banner ("Group · N new messages"), max 1 concurrent banner.

6. **Configuration & Environment**:
   - `CHIBAN_VAPID_PUBLIC_KEY`: Base64 URL-safe VAPID public key.
   - `CHIBAN_VAPID_PRIVATE_KEY`: Base64 URL-safe VAPID private key.
   - `CHIBAN_VAPID_SUBJECT`: Contact URI (e.g. `mailto:admin@example.com`).
   - Integrated into `apps/api/internal/config/config.go` with startup validation and updated `.env.example`.

## Testing Decisions

### What Makes a Good Test
- Tests must verify observable user-facing outcomes rather than internal plumbing:
  - Subscribing with valid VAPID keys creates an authenticated subscription record.
  - Subscribing with an existing endpoint previously owned by another user reassigns ownership cleanly without unique key errors.
  - Rotating an endpoint for an existing `(user_id, device_id)` replaces the old endpoint without duplicate key violation.
  - Unsubscribing removes the caller's subscription record and cannot delete another user's record (IDOR prevention).
  - Endpoints with invalid schemes or non-allowlisted hosts are rejected with HTTP 400 (SSRF prevention).
  - Sending a new message triggers push dispatch to eligible members, not to the sender or devices actively holding an unexpired focus lease.
  - Sharing a single meal to multiple groups dispatches at most one push notification per recipient device.
  - Focus lease expires after timeout when heartbeat stops, naturally restoring push delivery.
  - An attacker attempting to spoof another user's `device_id` in focus events cannot mute that user's notifications.
  - Push service 403 errors do not delete subscription rows, while 404/410 terminal errors correctly purge them.
  - Idempotent send retries (same `client_message_id`) do not trigger duplicate push dispatches.
  - Logging out with `device_id` deletes the subscription record for that device.

### Modules Tested
1. `apps/api/internal/push`: Unit tests for VAPID signing, payload packaging, SSRF host filtering, and error status code handling (403 vs 404/410).
2. `apps/api/internal/httpapi`: Integration tests for `/api/v1/push/*` endpoints, session checks, focus lease lifecycle & timeout, and group event notifications using the PostgreSQL test harness (`CHIBAN_TEST_DATABASE_URL`).
3. `apps/web`: Tests for Service Worker registration, IndexedDB device identity, `NotificationBanner` rendering/throttling, safe deep-link redirection, and `device_id` persistence.

### Prior Art
- `apps/api/internal/httpapi/chat_test.go`: Multi-user group chat broadcast and session isolation tests.
- `apps/api/internal/testsupport`: Isolated test harness with PostgreSQL migrations and session helpers.

## Out of Scope

- Third-party native push SDKs (Firebase Client SDK, OneSignal, APNs native binary bridge).
- SMS or Email notification fallbacks.
- Granular per-keyword muting or custom notification ringtone uploads.
- Reaction and delete events triggering push notifications (only chat messages and meal shares trigger alerts in V0.1.1).
- Rich interactive push actions (typing replies directly from OS lock screen banner).

## Further Notes

- iOS Push requirement: iOS 16.4+ requires users to install the web app to their home screen (`display: standalone`) before the Web Push API (`PushManager`) is exposed. An onboarding guide will assist iOS users with the "Add to Home Screen" step.
- VAPID private keys are durable secrets and must be backed up as part of deployment environment maintenance.
