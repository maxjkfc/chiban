# AGENTS.md

## Purpose

This repository contains **吃伴 (Chiban)**, a mobile-first shared diet-control MVP for small private groups.

This file defines repository-wide instructions for Codex and other coding agents. Product behavior and MVP scope are defined in [`docs/MVP_SPEC.md`](docs/MVP_SPEC.md).

## Source of truth

Before making implementation decisions, read:

1. `AGENTS.md` — engineering and agent rules.
2. `docs/MVP_SPEC.md` — product requirements, domain rules, MVP scope, API draft, and development phases.
3. Existing code, migrations, tests, and README — current implementation reality.

If these conflict:

- Explicit instructions from the current task take precedence.
- `docs/MVP_SPEC.md` is authoritative for product behavior and MVP scope.
- `AGENTS.md` is authoritative for repository-wide engineering constraints.
- Existing tests and migrations describe current behavior but must not silently override the product specification; call out material conflicts.

Do not invent missing product requirements. Choose the smallest implementation consistent with the specification.

---

## Product principles

The MVP exists to validate this loop:

```text
Eat
→ Record meal
→ See daily calorie impact
→ Share to group
→ Friends react / reply
→ Social accountability
→ Influence next meal
→ Record again
```

Core product rule:

> Record once, share automatically.

A meal is primary domain data. A chat message may reference a meal, but must not duplicate meal content, calories, or photo URLs.

---

## MVP scope discipline

Implement only requirements needed by the current phase or explicitly requested task.

Do **not** add these unless the specification is intentionally updated:

- AI food recognition or calorie estimation
- Apple Health / Health Connect
- push notifications
- native iOS / Android apps
- public social feed
- friend/follow system
- leaderboards or achievements
- payments or subscriptions
- Redis
- Kafka
- message queues
- microservices
- Kubernetes
- CDN
- signed image URLs
- unrelated infrastructure abstractions

Prefer boring, maintainable code over speculative architecture.

---

## Target architecture

The application is a **modular monolith**.

```text
apps/web        Next.js + TypeScript
apps/api        Go API + WebSocket server
migrations      PostgreSQL migrations
docs            product and architecture documentation
```

Runtime dependencies for the MVP:

```text
Next.js
Go API
PostgreSQL
fake-gcs-server
Cloudflare Tunnel
```

Local / Mac mini deployment should be reproducible with Docker Compose.

Do not split backend domains into independently deployed services.

---

## Backend boundaries

Expected Go modules/domains:

```text
internal/auth
internal/user
internal/profile
internal/goal
internal/tdee
internal/weight
internal/group
internal/meal
internal/chat
internal/storage
```

Rules:

- HTTP handlers parse/validate transport input and produce responses; they must not contain substantial domain logic.
- Domain/application logic belongs in services/use-cases.
- Database access belongs behind repository/query boundaries appropriate to the existing codebase.
- Avoid generic abstractions unless there are concrete consumers.
- Prefer explicit types over `map[string]any` for domain data.
- Pass `context.Context` through request-driven backend operations.
- Wrap errors with useful operation context; do not discard root causes.
- Do not log passwords, session secrets, tokens, private image paths, or sensitive health/profile data unnecessarily.

---

## Frontend rules

The web app is **mobile-first** and responsive.

Primary navigation:

```text
Today
Record
Groups
Me
```

Rules:

- Keep meal recording separate from normal chat composition.
- A meal can be shared to one or more groups after/because it is recorded.
- The group chat renders meal messages as structured cards referencing meal data.
- Frontend code must never depend on fake-GCS bucket names, object paths, or internal storage URLs.
- The frontend identifies stored meal images by application-level `image_id`.
- Prefer server/domain-derived authorization and validation over hiding controls in the UI.
- Do not duplicate backend business rules in a way that can diverge. Client-side calculations may be used for previews only when the server remains authoritative.
- Preserve accessible labels, keyboard behavior, focus states, and readable mobile layouts.

---

## Authentication and authorization

MVP authentication uses email/password.

Password requirements:

- Never store plaintext passwords.
- Use Argon2id or bcrypt according to the established implementation.

Session cookies should be configured with appropriate:

```text
HttpOnly
Secure (for production/public HTTPS)
SameSite
```

Authorization is a backend responsibility.

Every protected resource must verify the authenticated user and relevant ownership/group membership. Examples:

- group messages require membership in the group;
- meal edits/deletes require ownership;
- meal-image reads require permission to view the underlying meal;
- reaction/reply operations require access to the parent message/group.

Never rely on a client-provided user ID for ownership decisions.

---

## Meal and chat domain rules

### MealRecord

`MealRecord` is primary data.

Required MVP fields include at least:

```text
user_id
meal_type
eaten_at
```

Nutrition fields are optional in the MVP unless later requirements make them mandatory.

### MealPhoto

Database records store metadata, not binary image data.

The frontend must access images through application IDs, not storage paths.

### ChatMessage

MVP message types:

```text
text
meal
image
system
```

A meal chat message references:

```text
meal_record_id
```

It must not copy the meal description, calories, or image URLs into the message record.

### Replies

Do not create a separate comment subsystem.

A comment on a meal is a chat reply implemented with:

```text
reply_to_message_id
```

### Reactions

Reactions belong to messages and should enforce the uniqueness rule defined in the MVP specification.

---

## TDEE rules

MVP BMR/TDEE calculation uses the **Mifflin–St Jeor** formula as defined in `docs/MVP_SPEC.md`.

Important rules:

- Treat TDEE as an estimate in product copy.
- Server-side calculation is authoritative.
- Preserve calculation history in `tdee_calculations`; do not overwrite history with only the latest number.
- Persist calculation inputs and `formula` / `formula_version` so historical values remain explainable.
- Recalculate when specified profile/goal inputs change.
- Add unit tests covering formula branches, activity multipliers, goal adjustment, and representative edge cases.

Do not silently change the formula or activity factors without updating the specification and tests.

---

## Object storage

MVP object storage is `fake-gcs-server` running in Docker.

The backend should depend on a small storage interface, for example conceptually:

```go
type ObjectStorage interface {
    Upload(...)
    Delete(...)
    Open(...)
}
```

Do not expose fake-GCS directly to the browser.

MVP image read path:

```text
Browser
→ Go API /api/v1/meal-images/{image_id}
→ authenticate
→ authorize against meal/group access
→ read object from fake GCS
→ stream response
```

Never accept an arbitrary object path from the client and proxy it blindly.

Storage volume data must be persistent across container recreation.

---

## Database and migrations

Database: PostgreSQL.

Rules:

- All schema changes must use migrations.
- Never edit an already-applied migration merely to change history; add a new migration unless the project is explicitly still in an unshared bootstrap state.
- Add indexes/constraints for actual access patterns and integrity needs.
- Use foreign keys where they protect domain integrity.
- Use database uniqueness constraints for invariants such as membership or reaction uniqueness where applicable.
- Treat soft deletion consistently where the schema includes `deleted_at`.
- Avoid storing derived values in multiple tables unless the specification explicitly calls for a current cached value plus history.

Do not add another database technology for the MVP.

---

## API conventions

Base API prefix:

```text
/api/v1
```

Follow the API draft in `docs/MVP_SPEC.md` unless the current task intentionally revises it.

General rules:

- Validate input at the boundary.
- Return stable JSON response shapes.
- Use appropriate HTTP status codes.
- Do not leak internal storage paths, SQL errors, stack traces, or secrets.
- Pagination must be added before unbounded chat/history endpoints become large; for the first implementation, follow the current task/spec requirements rather than inventing a complex pagination scheme.
- Keep authorization checks close to application operations, not only in routing middleware.

WebSocket group endpoints must authenticate the user and verify group membership before joining the group broadcast room.

---

## Testing requirements

For every meaningful change:

1. Add/update unit tests for domain logic.
2. Add/update integration tests for API/database behavior when applicable.
3. Add regression tests for bugs being fixed.
4. Run formatting, linting, type-checking, and tests relevant to touched code.

High-priority test areas:

- TDEE calculation
- auth/session behavior
- group authorization
- meal ownership
- image authorization
- meal-to-chat-message sharing
- reply/reaction integrity
- WebSocket membership checks and broadcast behavior

Tests must not depend on production credentials or external paid services.

---

## Code quality

### Go

- Run `gofmt` on changed Go files.
- Prefer standard library facilities unless a dependency clearly reduces complexity.
- Keep interfaces small and consumer-driven.
- Avoid package-level mutable state except carefully owned runtime components such as the WebSocket hub.
- Prefer deterministic, testable functions for calculations.

### TypeScript / React

- Keep TypeScript strict where project configuration permits.
- Avoid `any` when a concrete type is practical.
- Prefer small components with clear data ownership.
- Do not use client state as a second source of truth for persisted domain entities.
- Follow existing formatting/lint/package-manager configuration once established.

---

## Docker and local development

The repository should eventually support a single documented startup path such as:

```bash
docker compose up
```

Expected services:

```text
web
api
postgres
fake-gcs
cloudflared (where configured)
```

Do not commit secrets or real credentials.

Keep `.env.example` updated whenever required environment variables change.

PostgreSQL and fake-GCS data should use persistent volumes/mounts.

---

## Backups

The MVP design assumes daily backup of:

- PostgreSQL data via `pg_dump` or equivalent documented process.
- fake-GCS persistent data.

Do not claim backups exist unless the scripts/configuration actually implement them.

---

## Agent workflow

For each task:

1. Read the relevant specification and surrounding code before editing.
2. Identify the smallest coherent change.
3. Preserve existing architectural boundaries.
4. Implement the change.
5. Add/update tests.
6. Run relevant verification commands.
7. Update documentation when behavior, setup, environment variables, or architecture changes.
8. Summarize what changed and any unresolved risk.

Do not perform broad refactors unrelated to the task.

Do not silently change API contracts, database schemas, domain terminology, or MVP scope.

If a required decision is genuinely unspecified, choose the simplest reversible option and document the assumption in the change summary or relevant docs.

---

## Initial implementation order

Unless a task says otherwise, follow the phases in `docs/MVP_SPEC.md`:

1. Infrastructure / repository skeleton
2. Authentication
3. Profile / Goal / Weight / TDEE
4. Meal records and image storage
5. Groups
6. Chat / WebSocket / Reply / Reaction
7. Meal social sharing integration
8. Today page and calorie summary

The end-to-end MVP is not done until two users can join a group, one can record/share a meal with a photo, the other can see it in realtime and react/reply, and the first user's daily calorie summary reflects the meal.
