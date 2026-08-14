# CLAUDE.md

## Project

This repository contains **吃伴 (Chiban)**, a mobile-first shared diet-control MVP for small private groups.

Claude Code should treat the following as the primary project references:

1. `CLAUDE.md` — Claude-specific operating guidance.
2. `AGENTS.md` — repository-wide engineering rules.
3. `docs/MVP_SPEC.md` — authoritative MVP product and technical specification.
4. Existing migrations, tests, code, and README — current implementation state.

If the current user task conflicts with repository documentation, follow the explicit current task and clearly note the intentional deviation when it changes product behavior, architecture, API contracts, or schema.

---

## Core product idea

The product is not a generic calorie tracker. It is a private-group diet accountability tool.

The core loop is:

```text
Eat
→ Record meal
→ See daily calorie impact
→ Share meal to group chat
→ Friends react / reply
→ Social accountability
→ Influence next meal
→ Record again
```

The key data-model principle is:

> Record once, share automatically.

`MealRecord` is primary data. A meal-type `ChatMessage` references the meal through `meal_record_id`; it must not copy meal calories, descriptions, or image URLs into chat storage.

---

## Before making changes

Before editing code:

1. Read the relevant sections of `docs/MVP_SPEC.md`.
2. Read `AGENTS.md`.
3. Inspect existing code in the affected module.
4. Inspect relevant tests and migrations.
5. Check whether the requested behavior already exists before adding parallel implementations.

Do not generate a second architecture alongside an existing one.

Do not rewrite unrelated code just because an alternative style is possible.

---

## Architecture

Use a **modular monolith**.

Target structure:

```text
apps/web        Next.js + TypeScript
apps/api        Go
migrations      PostgreSQL migrations
docs            specifications
```

Backend domain areas:

```text
auth
user
profile
goal
tdee
weight
group
meal
chat
storage
```

Runtime for the MVP:

```text
Mac mini
Docker Compose
Next.js
Go API / WebSocket
PostgreSQL
fake-gcs-server
Cloudflare Tunnel
```

Do not introduce microservices, Kubernetes, Redis, Kafka, or queue infrastructure for the MVP unless explicitly requested.

---

## How to make implementation decisions

Use this priority order:

1. Correctness and authorization.
2. MVP scope.
3. Simplicity and reversibility.
4. Testability.
5. Performance appropriate for a small private-group MVP.
6. Future scalability only where it does not materially complicate the MVP.

Avoid speculative abstractions.

A useful abstraction is one that isolates a known infrastructure boundary, such as object storage. A speculative abstraction created for hypothetical future providers or services should generally be avoided.

---

## Go backend guidance

Keep HTTP transport, application logic, persistence, and infrastructure responsibilities distinguishable.

Handlers should primarily:

- decode requests;
- perform transport-level validation;
- call an application/service operation;
- map known errors to HTTP responses.

Do not put TDEE formulas, group membership policy, meal sharing rules, or storage authorization directly inside HTTP handlers.

Prefer explicit domain/application types.

Pass `context.Context` through request-scoped operations.

Wrap errors with enough context to diagnose the operation without exposing sensitive data to API clients.

Use transactions where a logical operation must remain atomic. In particular, consider transaction boundaries when creating records that must remain consistent, such as a meal and its associated share/message metadata.

Do not build distributed transaction machinery for the MVP.

---

## Next.js frontend guidance

The UI is mobile-first.

Primary navigation:

```text
Today
Record
Groups
Me
```

Meal recording is a distinct structured workflow; do not turn the chat composer into the primary meal-entry form.

Group chat should display meal records as structured message cards.

Frontend rules:

- Do not expose or depend on fake-GCS internal URLs or object paths.
- Use application-level IDs for images and resources.
- Treat server authorization as authoritative.
- Avoid reproducing backend business rules as independent sources of truth.
- Use client-side previews only where useful, while server results remain authoritative.
- Maintain accessibility for forms, navigation, buttons, dialogs, keyboard focus, and mobile layouts.

---

## TDEE implementation

MVP uses the **Mifflin–St Jeor** formula and activity factors specified in `docs/MVP_SPEC.md`.

Do not substitute a different formula without an intentional specification change.

Requirements:

- calculate on the backend;
- return/display TDEE as an estimate;
- persist calculation inputs;
- persist `formula` and `formula_version`;
- preserve historical calculations;
- recalculate on specified profile/goal changes;
- cover male/female formula branches, activity factors, goal adjustment, and representative edge cases with tests.

Avoid using only a mutable `tdee` column as the sole record of calculation history.

---

## Authentication and authorization

Authentication is email/password for the MVP.

Never store plaintext passwords.

Use the established password hashing implementation (Argon2id or bcrypt).

Do not trust client-provided user IDs for ownership.

Every protected operation must derive the authenticated user from the session/auth context.

Authorization examples:

- user may edit/delete only their own meals;
- group chat requires group membership;
- joining WebSocket rooms requires membership validation;
- meal-image reads require permission to view the related meal;
- reactions/replies require access to the referenced message/group.

Security checks must be enforced in backend operations even if the UI also hides unauthorized controls.

---

## Image storage

The MVP uses `fake-gcs-server` as object storage.

Storage must remain behind an application/backend abstraction.

Browser-facing read flow:

```text
Browser
→ GET /api/v1/meal-images/{image_id}
→ Go authenticates user
→ Go checks meal/group authorization
→ Go resolves bucket/object internally
→ Go reads from fake GCS
→ Go streams image
```

Do not implement endpoints that accept arbitrary GCS paths from the browser.

Do not expose the fake-GCS service publicly.

The storage volume must persist across container recreation.

Future signed URLs or real GCS are out of MVP scope unless explicitly requested.

---

## Meal/chat consistency

Meal creation and social sharing are related but distinct concepts.

A meal may exist without being shared.

When shared to a group, create a chat message that references the meal:

```text
message_type = meal
meal_record_id = <meal id>
```

Never copy the mutable meal fields into the chat record merely for rendering convenience.

Do not implement a separate comment table for meal discussions.

Replies use:

```text
reply_to_message_id
```

Reactions attach to chat messages.

---

## PostgreSQL and migrations

Use PostgreSQL for MVP relational data.

Every schema change must be represented by a migration.

Use constraints to protect invariants where practical:

- foreign keys;
- ownership relations;
- unique group membership where applicable;
- unique reaction tuple as specified;
- required fields and sensible nullability.

Do not add MongoDB, Redis, Elasticsearch, or another persistence layer for MVP convenience.

Do not store image binaries in PostgreSQL.

---

## Realtime chat

MVP realtime chat uses the Go process and WebSockets.

Expected flow:

```text
Client sends message
→ backend validates membership/input
→ persist message
→ broadcast to connected members of the group room
```

A single-node in-memory WebSocket hub is acceptable for the MVP.

Do not add Redis Pub/Sub or multi-node coordination until the runtime actually needs multiple API instances.

WebSocket connections must authenticate and authorize before joining a room.

---

## Testing expectations

Changes are not complete merely because the code compiles.

For relevant changes:

- add/update unit tests;
- add API/database integration tests where appropriate;
- add a regression test for bugs;
- run relevant test suites;
- run formatting/lint/type-checking for changed code.

Critical behavior to cover thoroughly:

```text
TDEE calculations
auth/session
meal ownership
group membership
image authorization
meal sharing → chat message
reply/reaction integrity
WebSocket membership and broadcasts
```

Tests must remain runnable without production credentials or paid external services.

---

## Documentation

Update documentation when changing:

- environment variables;
- Docker services;
- startup commands;
- API contracts;
- architecture decisions;
- TDEE rules;
- database behavior;
- MVP scope.

Do not leave `docs/MVP_SPEC.md` describing behavior that the implementation intentionally changed without also updating the specification.

---

## Environment and secrets

Never commit:

- passwords;
- API secrets;
- production session keys;
- Cloudflare tunnel secrets;
- database credentials intended to remain private.

Keep `.env.example` synchronized with required configuration and use placeholder values.

Do not expose PostgreSQL or fake GCS publicly.

---

## Scope guardrails

Unless explicitly requested, do not implement:

```text
AI meal recognition
AI calorie estimation
health-platform integrations
push notifications
native mobile apps
public social network features
friends/follows
leaderboards
achievements
payments
subscriptions
nutritionist portals
Redis
Kafka
microservices
Kubernetes
CDN
signed URLs
```

If a task appears to require one of these, first verify whether it is truly necessary to satisfy the requested behavior or whether the existing MVP architecture can solve it more simply.

---

## Working style for Claude Code

When given a development task:

1. Inspect before editing.
2. State internally the files/modules that actually need changes; avoid broad repository churn.
3. Implement the smallest coherent solution.
4. Preserve domain boundaries.
5. Add tests alongside behavior.
6. Run relevant verification commands.
7. Review the diff for unintended changes.
8. Update docs when required.
9. Summarize implementation, verification performed, and remaining caveats.

When multiple valid approaches exist, prefer the one that is easiest to understand and remove later.

Do not leave placeholder implementations, TODO-only handlers, fake success responses, or silently skipped authorization in work presented as complete.

---

## Development order

Unless the current task explicitly changes priority, follow:

1. Infrastructure / repository skeleton
2. Authentication
3. Profile / Goal / Weight / TDEE
4. Meal records and image storage
5. Groups
6. Chat / WebSocket / Reply / Reaction
7. Meal-to-chat social sharing
8. Today page / calorie summary

Refer to `docs/MVP_SPEC.md` for acceptance criteria and the complete MVP Definition of Done.
