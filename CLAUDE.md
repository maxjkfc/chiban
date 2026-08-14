# CLAUDE.md

## Project

**吃伴 (Chiban)** is a mobile-first private-group meal recording and social interaction MVP.

Before changing product behavior, read:

- [`docs/MVP_SPEC.md`](docs/MVP_SPEC.md) — authoritative V0.1 product and technical scope.
- [`AGENTS.md`](AGENTS.md) — repository-wide engineering constraints.

If current task instructions explicitly revise the spec, implement the requested change and update the relevant documentation in the same change.

## V0.1 objective

V0.1 validates:

```text
Record meal
→ Share to group
→ Friends see it realtime
→ Reply / React / GIF / Sticker
→ Social accountability
→ Record again
```

Do not turn V0.1 into a calorie tracker.

## V0.1 includes

- email/password authentication
- basic profile: display name, avatar, timezone
- private groups and invite links/codes
- meal recording with 1–4 photos
- meal time, optional meal type and note
- Today / history views without calorie calculation
- meal sharing to groups
- realtime WebSocket chat
- reply and reaction
- chat image upload
- GIF upload/render
- user-created stickers, including GIF stickers
- PostgreSQL
- fake-gcs-server
- Next.js frontend
- Go modular-monolith backend
- Docker Compose deployment on a Mac mini

## Not V0.1

Do not implement unless the requirement is explicitly updated:

- BMR / TDEE
- calories / macros
- weight tracking
- AI food or calorie analysis
- Apple Health / Health Connect
- push notifications
- public feeds or follow systems
- GIF search providers such as GIPHY/Tenor
- image timestamp watermarks
- Redis / Kafka / queues
- microservices / Kubernetes
- CDN / signed URLs

## Critical domain invariants

### Meal is primary data

A `MealRecord` exists independently of chat.

A meal chat message references `meal_record_id`; do not copy meal description, media paths, or future nutrition fields into durable chat-message fields.

### Sharing is explicit

Use `meal_group_shares` as the source of truth for whether a meal is shared with a group.

Do not infer visibility solely from the existence of a chat message.

### Replies are chat messages

Do not create a separate comments domain. Use `reply_to_message_id`.

### Chat retries are idempotent

Messages must carry a client-generated `client_message_id`. Persist before broadcasting and prevent duplicate inserts on retry.

### Timezone matters

Persist timestamps as `timestamptz` / UTC and use the profile IANA timezone for Today/history grouping.

### Deleted content preserves conversation context

Meal and message deletions should use the soft-delete/tombstone behavior described in the spec. Do not cascade-delete reply threads.

## Media rules

All object storage goes through the Go backend storage abstraction.

The frontend must never use fake-GCS bucket names, internal object paths, or internal storage URLs as durable identifiers.

Use application IDs such as:

```text
image_id
media_id
sticker_id
```

For protected media reads:

```text
Browser
→ Go API
→ authentication
→ authorization
→ fake GCS
→ stream
```

Never accept arbitrary object paths from the browser for proxy reads.

Static images should be validated, size-limited, dimension-limited, and stripped of EXIF/GPS metadata. GIFs must preserve animation.

Do not add image time watermarks in V0.1.

## Storage consistency

PostgreSQL and fake GCS are separate systems.

When an upload succeeds but DB persistence fails, perform best-effort object cleanup. Do not share/publish an incomplete Meal. Keep the implementation simple; no queue is required for V0.1.

## Backend style

Use a modular monolith, not microservices.

Expected domains include:

```text
auth
user
profile
group
meal
chat
media
sticker
storage
```

Keep HTTP handlers thin. Put business logic in application/domain services. Keep storage and persistence concerns behind clear boundaries consistent with the existing code.

Use `context.Context` for request-driven Go operations. Preserve useful error context without leaking internal details to API clients.

## Frontend style

The UI is mobile-first.

Primary navigation:

```text
Today
Record
Groups
Me
```

Meal recording is a dedicated primary action. Do not make users enter structured meal data through the normal chat text field.

Keep the fastest valid record flow close to:

```text
Take/select photo → Publish
```

## Authorization checklist

Always check backend authorization for:

- joining/reading/sending in a group
- WebSocket group subscription
- meal edit/delete/read/share
- meal-image read
- reply/reaction operations
- sticker/media mutations

Never rely on frontend state or client-supplied user IDs for authorization.

## Database

PostgreSQL only for V0.1 relational data.

All schema changes use migrations. Prefer DB constraints for real invariants such as unique group membership, meal shares, and reactions.

Do not add speculative future nutrition columns to `meal_records` just because TDEE/calorie work is planned later.

## Testing expectations

Add/update tests when behavior changes.

Prioritize tests around:

- authentication/session handling
- group membership/invites
- meal ownership/sharing
- media authorization
- timezone grouping
- WebSocket authentication and persistence-before-broadcast
- chat retry idempotency
- reply/reaction integrity
- upload failure cleanup

Run the relevant Go and web formatting/lint/type/test commands before considering work complete.

## Change discipline

For a task:

1. Inspect relevant files before editing.
2. Make the smallest coherent change.
3. Do not broaden scope unless required.
4. Preserve existing domain boundaries.
5. Add tests for changed behavior.
6. Update docs/env examples if behavior/setup changes.
7. Report what changed and any known limitation.

Do not silently change API contracts, schema semantics, or MVP scope.

## Phase order

Unless the task says otherwise:

1. Infrastructure
2. Authentication + Basic Profile
3. Group + Invite
4. Meal + Photo
5. Chat Core
6. Meal Sharing
7. Chat Media + GIF + Sticker
8. UX Polish / MVP Validation
