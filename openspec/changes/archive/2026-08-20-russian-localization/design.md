## Context

`FoodItem` and `CustomFood` currently each have a single `Name string` column (see
`backend/pkg/database/models_food.go`). Recognition (`backend/pkg/vision`) asks the model for one
English name per item and that name flows straight into `Name`, into USDA/OFF candidate retrieval,
and into custom-food matching (`food-photo-recognition` "Macro Estimate Fallback", `usda-nutrition-
database` "Match Selection").

The per-user settings store (`UserSettings.SettingsJSON`, `GET/PUT /api/users/me/settings`) already
exists and is implemented — this was the "other stream" the Display Language setting was waiting on.
It's a fully opaque JSON blob, so adding `display_language` is additive: no migration, no new table.

The frontend is a Next.js static export (`output: 'export'`) with no existing i18n library and no
`[locale]` route segment — routes are flat (`app/food`, `app/data`, `app/login`, ...).

`backend/pkg/database/db.go` uses `AutoMigrate`, which only adds columns/tables — it never renames
or rewrites existing ones (see the comment at `db.go:82`). This shapes the schema decision below.

## Goals / Non-Goals

**Goals:**
- A user can set a Display Language and see the UI and AI-recognized food names in it.
- An English "ground truth" name (Canonical Name) survives alongside the Display Name for every
  recognized item, retrievable via Expert Mode.
- Non-English recognition never silently produces wrong/mismatched reference-DB matches.

**Non-Goals:**
- Translating already-logged historical items when a user changes their Display Language later.
- Localizing the manual food-search bar's query translation (`food-search-translation`) — already
  handles arbitrary-language input for a different flow (typed search vs. photo recognition).
- Full app-wide i18n coverage of every string on day one — static UI strings are translated
  incrementally; the mechanism just needs to support it (see Decisions).
- A general-purpose fuzzy-search feature; the fuzzy match introduced here is scoped to custom-food
  reuse during recognition, against one user's own (small) catalog.

## Decisions

### 1. Schema: add `CanonicalName`, keep `Name` as the Display Name column

Rather than renaming `Name` → `DisplayName` (a rename `AutoMigrate` can't express safely — see
Context), add a new nullable `CanonicalName string` column to both `FoodItem` and `CustomFood`.
`Name` becomes, by convention, "the Display Name" — its Go doc comment is updated to say so, but
the column itself is untouched. This is the additive, zero-migration-risk path.

`CanonicalName` is empty for pre-existing rows (recognized before this change) and for any English-
Display-Language recognition where Display Name and Canonical Name are identical — no need to
duplicate storage when they're the same string. Expert Mode omits the Canonical Name sub-label
entirely when `CanonicalName` is empty, showing the Display Name alone rather than an empty or
placeholder field; see the expert-mode spec's "An item with no recorded Canonical Name shows only
its Display Name in Expert Mode". That is correct in both cases (nothing was lost; there was
nothing to show for a pre-existing English-only row anyway — that's the acceptable "VERY rare"
English fallback the proposal calls out). This paragraph previously described it as falling back
to *showing* `Name` in the Canonical Name position, contradicting both the spec scenario and
`CanonicalNameLabel.tsx`, which returns null. Corrected in code review.

Alternative considered: rename `Name` → `DisplayName` via a hand-written migration. Rejected —
this project has no migration framework beyond `AutoMigrate`, and introducing one for a single
rename is disproportionate at this scale.

### 2. Display Language storage: new key in the existing `UserSettings` blob

`display_language` (BCP-47-ish string, e.g. `"en"`, `"ru"`) is just another key in
`SettingsJSON`, read/written through the existing `GET/PUT /api/users/me/settings` endpoints. No
backend change needed beyond documenting the key in `user-settings`'s delta spec (see specs/).

### 3. Recognition: single vision call returns both names

The vision prompt (`backend/pkg/vision/openai.go`) is extended to ask for `display_name` (in the
caller-supplied target language) and `canonical_name` (English) per recognized item, in the same
JSON response it already returns — not two calls. The target language is passed into the
recognition call from the user's `display_language` setting.

Alternative considered: a second, separate translation call (mirroring `food-search-translation`'s
pattern). Rejected — recognition already returns structured JSON per item; adding two fields is
free within that call, whereas a second call doubles vision-provider cost and latency per meal
photo, and (per the proposal) a Display Name and Canonical Name for the same recognized item
should never disagree the way an independently-run translation could.

### 4. Open Food Facts/USDA querying skipped for non-English Display Language

`usda-nutrition-database`'s "Match Selection and Explicit Non-Match" requirement already branches
on whether a custom-food match was found before deciding whether to query Open Food Facts (brand
present) or USDA (no brand) for an item. This design adds one more gate to that same branch:
Open Food Facts and USDA SHALL NOT be queried at all when the recognizing user's Display Language
is non-English, regardless of brand — an item with no custom-food match in that case goes
straight to the macro-estimate fallback (`food-photo-recognition` "Macro Estimate Fallback for
Unmatched Items"), unchanged. The custom-food matching step itself (exact-turned-fuzzy, see
below, and the frequency/recency-ranked tier) is untouched by this gate — it always runs,
regardless of language, since it matches against the user's own catalog, not an English-vocabulary
reference database. Matched-item macro precedence, the estimated-macro fallback, and manual
reference binding via PATCH (`fdc_id`/`off_code`) are all otherwise unaffected — a user can still
manually bind a non-English item to a reference food after the fact if they want to.

### 5. Custom-food exact match becomes fuzzy: hand-rolled normalized Levenshtein, not a new dependency

`usda-nutrition-database`'s exact-name custom-food match (currently: "A case-insensitive exact
name match ... SHALL be offered as the sole candidate") becomes a case-insensitive **fuzzy** name
match, still offered as the sole candidate (still bypassing Open Food Facts/USDA) when it clears
the threshold. This is separate from, and unchanged from, the existing frequency/recency-ranked
custom-food tier that already runs when no exact/fuzzy name match is found.

Given catalog size (one user's own custom foods — realistically dozens, not thousands), this uses
a small hand-rolled normalized Levenshtein-ratio function (normalize: trim, lowercase, collapse
whitespace; then `1 - distance/max(len_a, len_b)` against every candidate, O(n·m) per pair, run
over the whole per-user catalog) rather than pulling in a fuzzy-matching library — consistent with
"don't add dependencies for small-scale problems" (see project CLAUDE.md). A threshold (proposed:
0.82 similarity) decides a match; when more than one candidate clears it, the highest-similarity
one is offered as the sole candidate (ties broken by most-recently-used). Below the threshold, no
exact/fuzzy candidate is offered and matching falls through to the existing frequency/recency-
ranked tier exactly as it does today for a non-matching name. The exact threshold is tunable
during implementation/testing, not fixed by this design.

Fuzzy matching runs against `Name` (the Display Name), since that's the language the user
actually recognizes their own catalog entries in.

### 6. Frontend i18n: static dictionary + React Context, not next-intl

Given the static export has no `[locale]` route segments and Display Language is a runtime,
per-authenticated-user setting (not a public/SEO'd URL concern), the frontend uses a small
hand-written translation-dictionary module (`en.ts`/`ru.ts` string maps) plus a React Context
provider that reads `display_language` from the settings API on load and exposes a `t()` lookup
function. Both dictionaries ship in every build (no dynamic per-locale chunk loading needed at
this scale).

Alternative considered: `next-intl`. Rejected for now — it's built around locale-prefixed routing,
which would mean restructuring every existing route under `[locale]/`, a large blast-radius change
for an app with two supported languages and a runtime (not URL-based) locale switch. Revisit if a
third language or SEO'd public pages are ever added.

### 7. Expert Mode: client-only, no persistence

Expert Mode is a plain React state toggle per screen — not written to `UserSettings`, not a URL
param. Reloading a page resets it to "off," matching the proposal's "per-screen, non-persisted"
decision.

## Risks / Trade-offs

- **[Risk]** Fuzzy-match threshold could cause false-positive reuse (e.g. matching "chicken soup"
  to "chicken salad") → **Mitigation**: threshold tuned conservatively (start high, ~0.82), and
  the match is only ever a *suggestion* surfaced to the user during confirmation, never
  auto-applied silently — same UX pattern as existing candidate suggestions.
- **[Risk]** `CustomFood`'s existing `uniqueIndex:idx_custom_food_user_name` is on `Name` (Display
  Name) — two custom foods with the same Display Name in different scripts can't collide (Russian
  vs. English text differs), but two recognitions of the same food in the same language could
  still hit the existing exact-uniqueness constraint when *creating* a new custom food, unrelated
  to the new fuzzy *matching* step → **Mitigation**: unchanged existing behavior, out of scope for
  this change.
- **[Trade-off]** Skipping reference-DB matching for non-English means non-English items rely
  entirely on the model's own estimated macros (`macro_source = estimated`) rather than a USDA-
  backed profile → accepted explicitly in the proposal ("Skip reference-DB matching for Russian").

## Migration Plan

- `AutoMigrate` adds `canonical_name` (nullable) to `food_items` and `custom_foods` — additive,
  no backfill needed (existing rows keep `canonical_name = ""`, correctly read as "no canonical
  name recorded").
- No `UserSettings` migration — `display_language` is just a new JSON key, absent means "unset"
  (design.md's frontend defaults unset to English).
- No feature flag needed: behavior for `display_language = "en"` (or unset) is unchanged from
  today, so this ships safely to existing users with no visible change until they opt in.

## Open Questions

None outstanding — all decisions above were confirmed during the grilling session or resolved by
inspecting the current codebase (see CONTEXT.md and the "already landed" `user-settings`
dependency).
