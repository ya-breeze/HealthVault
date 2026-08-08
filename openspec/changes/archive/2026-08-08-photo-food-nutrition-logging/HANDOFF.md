# Handoff: HealthVault photo-food-nutrition-logging

The core feature is done, dogfooded, and its OpenSpec change is **archived** (merged into
`openspec/specs/{food-nutrition-logging,food-photo-recognition,usda-nutrition-database,
food-model-calibration}/spec.md` plus updates to `data-api`, `data-model`, `record-deletion`).
This file replaces the prior handoff, which lived at
`openspec/changes/photo-food-nutrition-logging/HANDOFF.md` before archiving moved the whole
directory here.

**Branch:** `feat/photo-food-nutrition-logging` (PR #4, **draft — do not merge**)
**HEAD at this handoff:** the archive commit on top of `dcd924e` (check `git log` for the exact
SHA — it's whatever committed the `openspec archive` output described below)

Photo-based meal logging: upload a photo → OpenAI Vision recognizes foods and gram weights →
matched against a local USDA FoodData Central SQLite/FTS5 index → user reviews, can correct any
item (matched or not), and confirms. Plus custom user foods, manual (photo-free) entry, and an
operator-run model calibration CLI (not built — see "Not done").

## Build
```
make build   # or: cd backend/cmd && go build -tags sqlite_fts5 -o ../bin/hcw .
make test    # go test -tags sqlite_fts5 ./...
make lint    # go vet -tags sqlite_fts5 ./...
```
`-tags sqlite_fts5` is mandatory. Without it everything compiles and then fails at runtime with
`no such module: fts5`.

## THE most important open item: tier-2 review has never run on this branch

`/code-review` (tier 2 — the review that actually counts, since it's blocked from agent
invocation) has **never successfully completed** against this branch's real diff, across two
sessions:
- Session 1: `/code-review low` diffed `HEAD` against its own already-pushed upstream tracking
  branch (identical), not against `main` — returned empty, meaninglessly.
- Session 2 (this one): the change was **archived before tier 2 ran**, on the user's explicit
  choice ("archive now anyway, review the archived HEAD after") rather than the default order
  (tier 2 → approve → archive). This is a valid path per `/data/CLAUDE.md`'s own step 9 ("review
  the final HEAD"), but it means the gate is still pending, not satisfied.

**Do not treat this PR as reviewed.** Two rounds of *agent* (tier-1) review did run this session —
see "What tier-1 review already covered" below — but per `/data/CLAUDE.md`, tier-1 is explicitly
weaker and does not substitute for tier 2. Get the user to run `/code-review` on this exact
archived HEAD, address whatever it finds, and re-review after any fix before merging.

## What tier-1 review already covered this session

Two fresh-subagent review passes ran against the exact commits, findings fixed and re-reviewed:
1. Review of `095c95b..1bc8009` (the two dogfood bug-fix commits) — found one real gap: a
   name-only `PATCH .../items/{item_id}` with an empty/whitespace name silently no-op'd past the
   "nothing to update" 400 guard instead of being rejected. Fixed, tested
   (`TestPatchMealItem_BlankNameAloneReturns400`), and cross-referencing comments added between
   `nginx/nginx.conf` and `backend/pkg/server/food_upload.go` so a future bump of
   `HCW_MAX_UPLOAD_BYTES` can't silently reintroduce the 413 bug (see "Dogfood bugs fixed" below).
2. Review of the resulting fix-up commit (`dcd924e`) — clean, no further issues.

## Dogfood bugs fixed this session (real usage caught these, not tests)

1. **413 on photo upload.** `nginx/nginx.conf`'s `/api/` location had no `client_max_body_size`,
   so it inherited nginx's 1MB default and rejected phone-camera-sized photos before they ever
   reached the backend's own 10MB (`HCW_MAX_UPLOAD_BYTES`) limit. Fixed: `client_max_body_size
   12m;`, with a comment tying it to the backend's configured cap.
2. **"Take Photo" crashed the page.** `CameraCapture.tsx` called
   `navigator.mediaDevices.getUserMedia(...)` unconditionally. `navigator.mediaDevices` is
   `undefined` outside a secure context (HTTPS or `localhost`) — verified via Playwright
   (`window.isSecureContext` is `false` on `http://192.168.1.54:8888`, `true` on
   `https://192.168.1.54:9888`, which already has a self-signed cert from `nginx/Dockerfile`).
   Calling `.getUserMedia` on `undefined` throws *synchronously*, before any promise the
   component's `.catch()` could handle — it crashed the whole page to Next.js's error boundary.
   Fixed with a guard that sets the existing friendly error state instead. **The camera still only
   works over HTTPS** — that's a real browser requirement, not fixable in app code. Tell users to
   use `https://192.168.1.54:9888` (self-signed cert, one-time browser warning to accept) if they
   want in-app camera capture; `http://192.168.1.54:8888` now fails gracefully instead of
   crashing, but still won't open the camera.
3. **Wrong vision match had no correction path.** The review UI only offered "Resolve this item"
   when `macro_source = none`. If vision matched something *wrong* (real example hit live: "dark
   berries" guessed for what were actually cherries) there was no way to fix it — the backend
   never actually restricted `PatchMealItem` by current `macro_source`, only the frontend gated
   it. Fixed: the resolve UI is now reachable for any item ("Change match" when already resolved);
   binding to a search result syncs the item's displayed `name` to the match; manual mode gained
   an editable name field; item names now wrap instead of single-line-truncating (this was also
   reported directly — the AI's longer food guesses were getting cut off with no way to read them).
   This was scope beyond the original spec's "unresolved-item review UI" framing, so it went
   through `opsx:update` (spec/design/tasks revised, user confirmed) before implementation — see
   the archived spec's Item Resolution requirement for the final wording.

## Not done — Task 8 (operator calibration CLI) and its UI

Deliberately deferred, not needed for the feature to work; a separate operator tool for
benchmarking vision models against weighed ground-truth photos. Still unchecked in the archived
`tasks.md` (archiving with unchecked tasks is allowed by the tooling; the user chose to ship
without it rather than split it into a follow-up change first).

- [ ] 8.1 `FoodCalibrationSample` model, migration, tenant-scoped queries, photo cleanup on delete
      (the model itself already exists in `models_food.go` with correct tags — this task is the
      query/handler layer around it)
- [ ] 8.2 Authenticated create/list/delete calibration sample endpoints — create no `FoodMeal`
      records
- [ ] 8.3 `hcw calibrate-food-models` CLI: dataset selection, dry-run vs. confirmed execution,
      runtime model overrides, repeated trials, operator-supplied pricing
- [ ] 8.4 Deterministic ground-truth matching, detection/weight accuracy metrics, latency/cost
      aggregation, threshold selection, Pareto-frontier selection, reproducible JSON/Markdown
      reports
- [ ] 8.5 Tests for all of the above
- [ ] 7.7 Calibration sample capture/management UI — blocked on 8.2

If picked up later, this needs its own new OpenSpec change (this one is archived).

## Backlog raised by the user during dogfooding — not part of this change, no spec yet

Also saved to memory (`project_healthvault_food_backlog.md`) so a future session has this context
without re-deriving it:

1. **A "pending meals" link.** No dashboard entry point to a meal awaiting review/confirm — the
   only way in is the direct `/food/review/?meal=<uuid>` URL from the upload flow itself. Closing
   the tab mid-review currently loses the path back in except by remembering/re-finding the UUID.
2. **A page to browse/edit already-confirmed meals.** `ReviewClient.tsx` renders items `readOnly`
   once `confirmed`; only the generic `/data/food_meal` raw-table view exists for confirmed meals.
3. **Let the user give vision a free-text hint and re-parse when it completely misses.** Neither
   `RetryMeal` (re-runs blind, same photo, no new info) nor `ClarifyMeal` (only answers questions
   the *model* posed) covers "no, this is actually X" as an open channel. Recommended shape:
   optional hint/comment field alongside retry, appended into the vision prompt for that call.

Each is plausibly its own OpenSpec change.

## Decisions that are settled — do not silently reverse

Nutrition table never written, synchronous vision call, retry only on failed/stale-processing,
`MacroSource` tri-state, HEIC rejected, candidate shortlist N=30, preparation/state as ranking
hints never filters, retrieval never auto-assigns, manual meals created already `confirmed`
(aggregate computed immediately), photo-based meals leave the aggregate at zero until
`PUT .../confirm`, `ClarifyMeal` transitions through `processing` (concurrency fix), pending
clarification questions live inside `clarify_log` itself (unanswered = empty `Answer`), `RetryMeal`
resets `clarify_round`/`clarify_log` to zero. New from this session: **an item's `name` can be
corrected independently of (or alongside) rebinding it** — `PATCH .../items/{item_id}` accepts an
optional `name`, applied regardless of which of manual/reference/weight-only branch runs, and
usable alone as long as it's non-blank after trimming.

## Codebase gotchas

- **`models.TenantModel` (kin-core) has no `BeforeCreate` hook.** Set `ID = uuid.New()` and
  `FamilyID` explicitly on every `Create()`.
- **GORM soft-delete + unique index is a trap.** `TenantModel` carries `gorm.DeletedAt`; a plain
  `.Delete()` soft-deletes. Any unique-indexed table needs `.Unscoped().Delete()` from the start —
  a `Count`/`Find`-after-delete test can't catch this, since it applies the same soft-delete filter
  as the bug it should detect. Assert on the actual regression (recreate the same key, or
  `.Unscoped()` the count) instead.
- **Struct JSON tags are not optional.** `FoodMeal`, `FoodItem`, `CustomFood` need `json:"..."` on
  every field they add, or responses emit PascalCase Go names instead of snake_case.
- **`:memory:` SQLite in tests gives each pooled connection its own separate database.** Any
  genuinely concurrent test needs `sqlDB.SetMaxOpenConns(1)` on the test storage. See
  `TestClarifyMeal_ConcurrentDoubleSubmitOnlyOneWins`. Production is unaffected (real file path).
- **All env vars use the `HCW` viper prefix.** Never introduce bare `OPENAI_API_KEY`.
- **`QueryRecords`** has a `columnAllowlist` map in `pkg/database/storage_impl.go` — `food_meals`
  is restricted to `id, logged_at, name, status` + the 7 macros, for both the HTTP path and MCP.
- **Frontend is a static export** (`output: 'export'`). Meal review is query-param routed:
  `/food/review/?meal=<uuid>`, not a `[id]` segment.
- **Ownership errors are 404, never 403.** Unauthenticated is 401. Consistent across every
  `/api/food/*` handler.
- **`navigator.mediaDevices` requires a secure context** (HTTPS or `localhost`) — `undefined`
  otherwise, and calling `.getUserMedia` on `undefined` throws synchronously, bypassing any
  `.catch()`. Guard with `navigator.mediaDevices?.getUserMedia` before calling, anywhere new camera
  code is added.
- **nginx `client_max_body_size` on `/api/` must stay above the backend's `MaxUploadBytes` +
  `multipartOverheadBytes`** (`backend/pkg/server/food_upload.go`) or uploads 413 before the
  backend's own limit is ever consulted. Both files now cross-reference each other in comments —
  keep that in sync if either limit changes.
- **Pre-existing, unrelated bug found in the main specs tree**: several `openspec/specs/*/spec.md`
  files are missing their `## Purpose` section (required by `openspec validate --specs --strict`).
  Fixed for the three this archive touched (`data-api`, `data-model`, `record-deletion`) since the
  archive command's rebuild-and-validate step blocked on it. **Still broken and untouched**:
  `authentication`, `health-connect-import`, `import-ui`, `libra-import`, `mcp-server`,
  `speed-data`, `webhook-ingest` — `openspec validate --specs --strict` will keep failing overall
  until someone adds `## Purpose` to those too. Unrelated to food logging; a separate cleanup.

## Deployment (hcw-wip)

- **Class is `dogfood`, not `wip`** (`data.json`) — it's the account's only deployed instance,
  `hcw.db` holds real irreplaceable data. No `--dry-run` on the `hcw` CLI.
- Currently deployed at `feat/photo-food-nutrition-logging`, port 8888 (`http://192.168.1.54:8888`)
  / port 9888 for HTTPS (`https://192.168.1.54:9888`, self-signed cert).
- **The user's own real account (`elias` / family "Kralovi") has a real `pending_review` meal**
  from testing this session — two items, "Pink yogurt or creamy dessert" (~220g) and "Dark
  berries..." (~40g), created 2026-08-08 ~08:28 UTC. **Do not touch it** — it's the user's data to
  confirm or delete themselves, not test residue. (E2E tests use the separate seeded `alice` /
  "TestFamily" account and clean up after themselves via the app's own DELETE endpoints, verified
  via `sqlite3 /data/data/hcw-wip/hcw.db` — that account was left with only its own leftover rows
  cleaned, not touched otherwise.)
- Compose env: `HCW_UPLOADS_DIR=/data/uploads`, `HCW_USDA_DB_PATH=/data/usda.db` (both under the
  stack's existing `/data` volume mount), `HCW_OPENAI_API_KEY` (real, live key — **do not print
  it**, it's in `data.json`), `HCW_OPENAI_MODEL=gpt-5.6-luna` (confirmed real via `GET /v1/models`,
  a custom-named model on this account, not a public OpenAI release name).
- USDA index already built and live: 7,793 foods, `HCW_USDA_DB_PATH=/data/data/hcw-wip/usda.db`
  from this container's perspective. Re-run `hcw import-usda` only if it needs refreshing, then
  redeploy so the running server picks up the new file (opened once at startup).
- **After merge**, this `dogfood` stack must be repointed to `main`:
  `/data/portainer.py git-deploy hcw-wip --branch main`, then update `branch` in `data.json`. Do
  not skip this — it's what "tracks main" means for a dogfood stack.

## Next steps, in order

1. User runs `/code-review` on this branch's current (archived) HEAD — the actual pending gate.
2. Fix whatever it finds; if anything changes HEAD, re-run `/code-review` on the new HEAD before
   proceeding (per `/data/CLAUDE.md`, a fix changes what "reviewed" means).
3. User approves merge. Merge to `main` **with squashing** (one commit for the whole change).
4. Repoint `hcw-wip` to `main` (see "Deployment" above) and update `data.json`.
5. Optionally: start new OpenSpec changes for Task 8 or any of the three backlog items above, when
   the user wants them.
