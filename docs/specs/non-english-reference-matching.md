# Fix USDA/OFF matching for non-English display languages

Idea: ya-breeze/idea-forge#640

## Why

`retrieveCandidates` (`backend/pkg/server/food_upload.go`) has skipped USDA/OFF candidate
search entirely for any user whose `display_language` isn't English, since the check was added
2026-08-20 (`if !vision.IsEnglishDisplayLanguage(displayLanguage) { return ranked, false }`).
Checked directly against production data before writing this: elias's account
(`display_language: ru`) shows a clean mix of `reference` and `estimated` items through
2026-08-29, then **100% `estimated`, zero `reference`, every single day through 2026-09-18** —
19 days with no exceptions. The gate has been silently discarding every reference-database
lookup for this account's entire recent food log.

The gate exists because USDA SR Legacy and Open Food Facts (the languages HealthVault searches
today) are English-language datasets, and a raw Cyrillic item name would never match them. But
the app already solves exactly this problem one layer up: `languageDirective`
(`vision/openai.go`) tells Recognize to write `display_name` in the user's language and
`canonical_name` as "the same food's standard English name" whenever the display language isn't
English (russian-localization design decision 3). Checked against the 257 currently-`estimated`
items: `canonical_name` is populated and clean for 232 of them ("Cottage cheese", "Blueberries",
"Apple", "Sugar snap peas", ...) — exactly the English text the search gate needs and already
has sitting unused on every one of these rows.

## How

Replace the early return in `retrieveCandidates` with a search-name choice: use `ri.Name` when
the display language is English (unchanged), and `ri.CanonicalName` when it isn't and is
non-empty. Only fall back to skipping search entirely when the display language isn't English
**and** `CanonicalName` is empty (an item recognized before this field existed, or one Recognize
genuinely couldn't translate) — that residual case is unchanged from today's behavior, just
narrowed to when it's actually unavoidable.

No schema change, no prompt change: `CanonicalName` is already generated and stored today. This
is a one-function fix plus the tests that prove it.

### Historical data

Out of scope for this change's code: the owner asked for a separate, deterministic backfill of
already-logged `estimated` items using their stored `canonical_name` (no new vision/LLM calls),
done directly against the running `hcw-prod` API rather than through this repo. Not part of this
PR.

## Validation Commands

- `make lint`
- `make test`
- `BASE_URL=http://192.168.1.54:8892 make test-e2e E2E_ARGS="tests/food-upload.spec.ts --retries=0"`

### Task 1: Search by canonical_name for non-English display languages

- [x] Replace `retrieveCandidates`'s early `!IsEnglishDisplayLanguage` return with a
      search-name choice (`ri.Name` for English, `ri.CanonicalName` for non-English), falling
      back to today's skip-entirely behavior only when `CanonicalName` is empty
- [x] Cover: non-English + populated CanonicalName finds OFF/USDA candidates; non-English +
      empty CanonicalName still skips search (unchanged); English is unaffected
- [x] Mark completed
