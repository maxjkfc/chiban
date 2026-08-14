# AGENTS.md

## Purpose

This repository contains **吃伴 (Chiban)**, a mobile-first private-group meal recording and social interaction MVP.

Read [`docs/MVP_SPEC.md`](docs/MVP_SPEC.md) before implementing product behavior. It is the source of truth for V0.1 scope, domain rules, APIs, and development phases.

## Current product goal

V0.1 validates one loop:

```text
Record a meal
→ Share it to a private group
→ Friends see it in realtime
→ Reply / React / GIF / Sticker
→ Social accountability
→ Record again
```

Core rule:

> Record once, share automatically.

`MealRecord` is primary data. Chat messages reference meals; they do not duplicate meal fields or storage URLs.

## V0.1 scope

Build only:

- email/password auth
- basic profile: display name, avatar, timezone
- private groups and invite flow
- meal record with 1–4 photos, time, optional meal type and note
- meal history / today view without calorie data
- meal sharing to groups
- realtime group chat
- reply
- reaction
- chat images
- GIF upload/render
- user-uploaded custom stickers, including GIF stickers
- fake-GCS-backed media storage

Explicitly **out of V0.1**:

- BMR / TDEE
- calorie tracking
- macros
- weight tracking
- AI food recognition / calorie estimation
- health integrations
- push notifications
- public social feed
- friend/follow system
- GIPHY/Tenor search integration
- image timestamp watermark
- Redis / Kafka / queues
- microservices / Kubernetes
- CDN / signed URLs

Do not add these without an explicit spec update.

## Architecture

Use a **modular monolith**.

```text
apps/web   Next.js + TypeScript
apps/api   Go API + WebSocket
PostgreSQL
fake-gcs-server
Cloudflare Tunnel
```

Suggested Go domains:

```text
internal/auth
internal/user
internal/profile
internal/group
internal/meal
internal/chat
internal/media
internal/sticker
internal/storage
```

Do not split domains into separately deployed services.

## Backend rules

- Keep substantial domain logic out of HTTP handlers.
- Validate input at transport boundaries.
- Pass `context.Context` through request-driven operations.
- Keep interfaces small and consumer-driven.
- Do not leak SQL errors, internal storage paths, secrets, or sensitive data.
- Authorization must be enforced by the backend, never only by frontend visibility.
- Never trust a client-provided `user_id` for ownership decisions.

## Time rules

- Store timestamps as PostgreSQL `timestamptz` / UTC.
- Store an IANA timezone on the profile, e.g. `Asia/Taipei`.
- Determine "today" and meal calendar dates using the user's timezone.
- Do not use server-local time as product truth.

## Meal rules

A V0.1 meal contains:

```text
user_id
meal_type nullable
eaten_at
description nullable
created_at
updated_at
deleted_at nullable
```

At least one photo is required; support up to four.

Do not add calorie/macro columns merely as placeholders for later phases unless the specification is intentionally revised.

## Meal sharing

Persist the sharing relationship independently of chat:

```text
meal_group_shares
meal_record_id
group_id
shared_at
revoked_at nullable
```

Do not use `chat_messages` as the source of truth for meal visibility.

When sharing a meal:

1. confirm the actor owns the meal;
2. confirm the actor can share into the target group;
3. create/retain the `meal_group_shares` relation;
4. create a `meal` chat message that references `meal_record_id`;
5. broadcast the persisted message.

## Chat rules

V0.1 message types:

```text
text
meal
image
gif
sticker
system
```

Comments are not a separate subsystem. Use:

```text
reply_to_message_id
```

for replies to any message, including meal messages.

Use a client-generated UUID `client_message_id` and enforce an idempotency constraint such as:

```text
(user_id, client_message_id)
```

Persist messages before WebSocket broadcast.

Chat history must use cursor pagination rather than loading all history indefinitely.

## Reaction rules

Reactions belong to chat messages. Preserve the uniqueness invariant defined in `docs/MVP_SPEC.md`.

## GIF and Sticker rules

- Direct chat GIFs are uploaded media; V0.1 does not integrate a GIF search provider.
- GIF animation must remain intact; do not flatten GIFs into static WebP.
- Users can upload and reuse their own stickers.
- Stickers may be static images or GIFs.
- Sticker chat messages reference `sticker_id`; never embed storage URLs as the durable model.

## Storage and media

Use `fake-gcs-server` through a backend storage abstraction.

Frontend code must never depend on bucket names, object paths, or fake-GCS URLs. Frontend uses application IDs such as `image_id`, `media_id`, and `sticker_id`.

Media reads go through the Go API:

```text
Browser
→ Go API
→ authenticate
→ authorize
→ fake GCS
→ stream response
```

Never proxy an arbitrary client-supplied storage path.

For static uploaded images:

- validate that data can actually be decoded as an allowed image;
- impose file-size and dimension limits;
- strip EXIF/GPS metadata;
- resize/compress where appropriate.

Do not add timestamp watermarks in V0.1.

## Storage/DB consistency

Object storage and PostgreSQL are not one transaction.

Handle partial failures explicitly:

- if upload succeeds but metadata persistence fails, best-effort delete the object;
- do not publish/share an incomplete meal;
- retain a simple orphan-cleanup path/script for stale objects.

Do not introduce a message queue solely for this in V0.1.

## Delete behavior

Use soft-delete/tombstone semantics where specified.

Deleting a meal must not cascade-delete an existing chat/reply thread. A meal message can render as a deleted-record tombstone.

Deleting a parent chat message must not destroy reply structure.

## Authentication and authorization

Passwords must be hashed with Argon2id or bcrypt.

Session cookies should use appropriate `HttpOnly`, `Secure`, and `SameSite` settings.

Protected operations must verify relevant ownership/membership, including:

- group message access
- group WebSocket join
- meal edit/delete
- meal read/share
- meal image read
- reply/reaction access
- sticker ownership for mutation

## Database and migrations

Database: PostgreSQL.

- All schema changes use migrations.
- Add foreign keys, unique constraints, and indexes where they enforce real invariants or access patterns.
- Do not add another database technology for V0.1.
- Do not rewrite migration history after it has been shared/applied; add a new migration.

## Frontend rules

The UI is mobile-first.

Primary navigation:

```text
Today
Record
Groups
Me
```

Meal recording is a dedicated flow, not just a chat composer mode.

The minimum recording path should approach:

```text
Take/select photo → Publish
```

Preserve accessible labels, keyboard/focus behavior, loading states, empty states, and recoverable error states.

## Testing

For meaningful changes, add or update tests.

High-priority areas:

- auth/session behavior
- group membership authorization
- invite lifecycle
- meal ownership
- meal sharing authorization
- image/media authorization
- storage/DB failure cleanup
- chat idempotency
- WebSocket membership and persisted-before-broadcast behavior
- reply/reaction integrity
- timezone-sensitive today/history behavior

Run relevant formatting, linting, type-checking, unit tests, and integration tests before declaring work complete.

## Docker and local development

The target local deployment is the user's Mac mini and should be reproducible through Docker Compose.

Expected services:

```text
web
api
postgres
fake-gcs
cloudflared (where configured)
```

Keep `.env.example` synchronized with required environment variables. Never commit real secrets.

PostgreSQL and fake-GCS data need persistent storage.

## Agent workflow

For each task:

1. Read `docs/MVP_SPEC.md` and relevant existing code.
2. Identify the smallest coherent implementation.
3. Preserve the modular-monolith and domain boundaries.
4. Implement the change.
5. Add/update tests.
6. Run relevant verification.
7. Update docs when setup, behavior, APIs, schema, or env vars change.
8. Summarize what changed and remaining risks.

Do not perform unrelated broad refactors or silently change domain terminology, API contracts, persistence semantics, or MVP scope.

## Implementation order

Unless explicitly changed by the current task, follow the V0.1 phases in `docs/MVP_SPEC.md`:

1. Infrastructure
2. Authentication + Basic Profile
3. Group + Invite
4. Meal + Photo
5. Chat Core
6. Meal Sharing
7. Chat Media + GIF + Sticker
8. UX Polish / MVP Validation
