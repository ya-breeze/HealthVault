# Issue tracker: Idea Store

Track this repository's work in Idea Store.
Use https://ideaforge.ikoro.in/ for owner-facing links.
Use the configured Store client for automated operations.

Canonical project key: `HealthVault`.

## Find and publish work

Resolve the repository's canonical project key before publishing.
Use the current project registry and the owning Idea's project field.
Ask the owner when those sources leave the project unresolved.
Read the full Idea, comments, Events and relevant Artefacts.
Publish a spec as an Idea or attach it to the existing owning Idea.
Keep repository specs under `docs/specs/` according to the environment rules.
Publish implementation tickets as child Ideas when decomposition is approved.
Record blocking edges as native Idea dependencies.
Keep parent hierarchy separate from dependency relationships.

## Store operations

Use `GET /api/ideas` to discover work.
Use `GET /api/ideas/{n}` to read the complete Idea and conversation.
Use `?view=row` only for narrow field checks.
Use `POST /api/ideas` to create an Idea.
Assign its verified project through `PATCH /api/ideas/{n}` after creation.
Use `POST /api/ideas/{n}/comments` for an agent-authored comment.
Identify Codex or Claude as the author in the comment text.
Use `POST /api/ideas/{n}/dependencies` with `depends_on` for a blocking edge.
Use the configured client to resolve the Store URL and authentication.

## Workflow

Follow the store-coordination skill before acting on a concrete Idea.
Read the current record immediately before each mutation.
Use supported Store operations and their concurrency protections.
Use stable idempotency keys when retrying creation or comment writes.
Verify the resulting record after a mutation.

A triage role never authorizes a lifecycle transition.
Preserve owner-controlled pauses and priority.
Follow project reservations, dependency gates and code-slot ownership.
Use supported owner actions for human decisions.
Let Reporting and Completion record verified final outcomes.

When a skill requests a triage label, use the interpretation in `triage-labels.md`.
Record the recommendation without creating a workflow tag or changing state.
Publishing a spec or child Idea does not establish implementation readiness.

## Code hosting

Use the forge for source code and PRs.
Keep work tracking in Idea Store for every repository.
Link each result PR to its owning Idea when one exists.
PRs as a request surface: no.
