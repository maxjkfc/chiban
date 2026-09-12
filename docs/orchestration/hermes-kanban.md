# chiban — Hermes Kanban Orchestration Rules (Hermes only)

> This file only applies to machines running this project under **Hermes**
> (`hermes kanban` CLI, worker profiles: max-coder/john-coder/reviewer-claude/
> reviewer-gpt, mini-pm orchestration). `max-coder`/`john-coder` here are the
> Hermes-specific names for the "backend agent" / "frontend agent" roles
> referenced in the root `AGENTS.md` — that mapping is defined only in this
> file, never in `AGENTS.md` itself.
>
> Machines running this project under a different framework can skip this
> file entirely — read only the root `AGENTS.md` (framework-agnostic rules),
> and define your own worker naming in your own framework's
> `docs/orchestration/<framework>.md`.

Ported from the japan-shopping-list project's proven orchestration rules (2026-09-08).

- **Issue-first workflow**: any user-facing "new feature"-level requirement must first get a
  GitHub issue (repo `maxjkfc/chiban`, label `enhancement`) with a problem statement, scope, and
  an Acceptance Criteria checklist, BEFORE kanban tasks are created. Every derived kanban task
  body must reference the issue (`GitHub Issue: #N`), and PR bodies use `Closes #N` (complete) or
  `Refs #N` (partial/dependency). Pure bug fixes / review-fix rounds tied to an existing PR/task
  chain do not need a new issue; a user-reported bug that needs independent tracking gets a `bug`
  issue first.
- **Review routing**: a PR's FIRST review round always gets both reviewer-claude and
  reviewer-gpt independently (never let one reviewer's summary leak into the other's task).
  Every subsequent "small fix" round responding to prior review comments only needs a single
  reviewer-gpt pass; Approve there merges directly. Only pull back to dual review if reviewer-gpt
  Request Changes or the issue looks significant/uncertain.
- **Event-driven review->merge chain, built in one shot at task-creation time, not
  backfilled after the fact**: whenever mini-pm creates a task that will produce/update a PR
  (implementer OR fix/repair task), it must in the SAME operation also create the follow-up
  review task(s) (`--parent <that task>`) and the decision/merge task (`--parent <review
  task(s)>`), and subscribe the decision task to notify (see below). Do not wait for the task to
  finish before deciding what comes next — pre-chain the whole
  dev/fix -> review -> decision/merge sequence up front so dispatcher auto-promotes every stage
  with zero manual follow-up. (Root cause of a real incident on japan-shopping-list: a fix task
  was created without its follow-up review pre-chained, and the PR sat "fixed but unreviewed"
  for ~2.5 hours until the user asked for a status update.)
- **Notify-subscribe every decision/rollup task**: `hermes kanban notify-subscribe <task_id>
  --platform discord --chat-id <chiban project channel/thread id> --delivery-mode notify` right
  after creating any decision/merge/rollup task — subscriptions are per-task, not per-board, so
  this must be repeated for every new decision task.
- **Board-wide safety-net cron, not a hardcoded task-id watchlist**: the monitoring cron script
  must scan the WHOLE board (`hermes kanban list`) for actionable signals — blocked tasks,
  completed review tasks needing a merge decision, and completed fix/repair tasks that may be
  missing their pre-chained follow-up review (safety net for the rule above) — rather than
  polling a fixed list of task IDs. A hardcoded ID list silently stops covering new tasks the
  moment they're created. Keep it wakeAgent-gated (script emits `{"wakeAgent": true/false}`) so a
  no-op tick costs $0. One merged cron per project (not one job per concern) is preferred to
  minimize scheduler wake-ups; 30 minutes is an adequate default interval since the real
  review->merge path is event-driven, not cron-driven — the cron is purely a backstop.
- **No standalone git-bot dispatch — dev workers commit/push/PR themselves** (2026-09-12,
  cross-project policy). git-bot has no unique capability (plain git operations the dev worker
  already does itself), adds dispatch queueing overhead, and cannot solve force-push blocks
  either (that's a system-level restriction on every profile, not git-bot-specific). Write
  "commit + push + open PR" directly into every max-coder/john-coder dev/fix task's own Scope/
  Acceptance Criteria; if a worker genuinely forgets, the orchestrator finishes it manually from
  that task's own worktree instead of dispatching git-bot.
