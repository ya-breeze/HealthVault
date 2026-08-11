## Context

Photo-recognition matching resolves each recognized item to a reference food via `backend/pkg/usda`: a local SQLite database with an FTS5 index over USDA FoodData Central (Foundation + SR Legacy, ~8k rows), built by an operator-run `import-usda` CLI command and queried through `Index.Search`/`Index.ByFdcID`. `FoodItem.FdcID *int64` and `FoodItem.CustomFoodID *uuid.UUID` already coexist as two parallel nullable reference columns — call sites in `food_item.go`/`food_manual.go` check `req.FdcID != nil && req.CustomFoodID != nil` (error: both set) and `req.FdcID != nil || req.CustomFoodID != nil` (has a reference).

USDA data is US-centric and doesn't reflect real European/Czech branded products. Open Food Facts (OFF) is a crowd-sourced, barcode-keyed product database with strong EU/CZ coverage, downloadable as a full nightly export (JSONL/CSV/Parquet, ODbL 1.0 license, attribution required).

## Goals / Non-Goals

**Goals:**
- Let photo-recognition candidates come from real Czech/EU-market product data (OFF) when available, falling back to USDA otherwise.
- Reuse the existing local-SQLite-index architecture (`usda` package) as the template for the new `off` package, so the two sources look and operate the same way to an operator.
- Make the change purely additive to the data model — no migration, no risk to existing `FdcID`/`CustomFoodID`-bound rows.

**Non-Goals:**
- Merging/ranking USDA and OFF candidates together in one bm25-scored list. Different FTS5 indexes have different corpora, so their bm25 scores aren't comparable; a fallback rule is used instead (see Decisions).
- Barcode-scan-driven food entry. `Index.ByCode` is built because binding a chosen candidate needs it, but no scan UI is added here.
- NutriDatabaze.cz (Czech generic-food database) — needs account registration and a license read a human has to do; stays a `todo.md` backlog note.
- Any change to how `CustomFood` precedence works (case-insensitive exact name match still wins over both USDA and OFF, unchanged).

## Decisions

### Third parallel nullable column, not a generalized Source+ExternalID

**Decision**: Add `FoodItem.OffCode *string` and `GroundTruthItem.OffCode *string` alongside the existing `FdcID`/`CustomFoodID`, rather than collapsing all three into a single `ReferenceSource string` + `ReferenceID string` pair.

**Why**: The codebase already carries two parallel nullable reference columns with pairwise exclusivity checks at each call site. A third column extends that exact pattern — one more nullable field, one more branch in each "at most one set" check — with zero migration of existing data. A generalized `Source+ExternalID` column would require backfilling every existing `FdcID` row with `source="usda"`, rewriting every lookup call site to branch on a string discriminator instead of a nil check, and gains nothing functionally: the discriminator is already implicit in which field is non-nil. This was raised and explicitly rejected during design review as unnecessary churn for the value it returns.

**Alternative considered**: `Source + ExternalID string` pair (what `todo.md`'s original research assumed would be needed). Rejected as disproportionate to a two-source (soon possibly three-source) system where "which column is non-nil" already answers "which source."

### OFF queried first, USDA only as a zero-result fallback

**Decision**: Candidate resolution in `food_upload.go` calls `off.Search` first. If it returns zero candidates, it then calls `usda.Search`. If OFF returns one or more candidates, USDA is not queried at all.

**Why**: bm25 scores are corpus-relative — merging OFF and USDA hits into one ranked-by-score list would compare numbers that don't mean the same thing across two different FTS5 indexes. A clean fallback rule is simple to reason about and to implement, and gives OFF's real Czech-market data precedence, which is the actual point of this change (the yogurt-protein example: a real CZ product's label beats a generic USDA estimate).

**Accepted limitation**: OFF is a branded/packaged-product database and is weak on generic staple foods ("rice", "chicken breast", homemade dishes) — the same caveat raised in `todo.md`'s original research. A low-relevance OFF match on a generic query still counts as "OFF returned candidates," blocking the USDA fallback and potentially hiding a better USDA match. This is accepted for now; revisit (e.g. a minimum-candidate-quality threshold instead of a strict zero-result trigger) only if it proves to matter once dogfooded.

**Alternative considered**: always query both and interleave top-N from each with a visible source badge (considered during design review, not chosen — the user picked the simpler EU-first/US-fallback rule over interleaving both source's results on every search).

### Country/completeness filter on OFF import, mirroring the USDA import's own scope limit

**Decision**: `import-off` filters the OFF export to products whose `countries_tags` include Czech Republic and/or Slovakia (extendable to EU broadly) and whose `nutriments` contain complete calories/protein/carbs/fat, before writing to the local SQLite+FTS5 index.

**Why**: The full OFF export is 3.5M+ globally-sourced branded products of uneven quality. Importing all of it would make the local index huge and mostly irrelevant to a Czech user, and would slow every search. This mirrors how `usda`'s importer only takes Foundation + SR Legacy (~8k curated rows) rather than the full FDC Branded Foods dataset. `off.Builder.Promote` reuses the same `MinExpectedRows`-style guard as `usda.Builder` so a truncated/bad filtered import never replaces a working database.

**Open sub-decision left to implementation**: exact `countries_tags` list and whether to widen to all EU country tags versus CZ+SK only — start narrow (CZ+SK), widen later if useful, since narrowing the corpus is strictly the safer failure mode (fewer, higher-confidence matches) versus widening it.

### `off` package structure mirrors `usda` package structure

**Decision**: `backend/pkg/off` reimplements `Index.Search(term, limit)`, `Index.ByCode(code string)`, `Builder`/`Add`/`Promote`/`Discard`, and the same `sanitizeFTSQuery` OR-join approach, as a parallel package rather than a shared generic abstraction over both.

**Why**: The two sources have genuinely different schemas (OFF has `code`/`brands`, no `data_type`; nutrient completeness and filtering rules differ). Two small, independently-buildable packages that happen to look similar are easier to reason about and modify independently than one generic package parameterized over both, especially since the import/filter logic (the part most likely to need tuning per source) is exactly where they differ most. `usda.go`'s existing doc comments about bm25 length bias, bm25 corpus-relativity, and bare punctuation causing FTS5 syntax errors apply unchanged to the new package's FTS5 usage and don't need to be relearned.

## Risks / Trade-offs

- **[Risk]** A mediocre OFF match on a generic-food query blocks the USDA fallback (see "accepted limitation" above). → **Mitigation**: none built now; flagged as a known trade-off to revisit if it shows up in real usage. The review UI still shows which source a match came from, so a bad OFF match is visible and correctable by hand even when the fallback doesn't trigger.
- **[Risk]** OFF's ODbL 1.0 license requires attribution, and share-alike terms apply if the *built* filtered database were itself redistributed publicly. → **Mitigation**: the built SQLite index is operator-local data (like the USDA one), not committed to the repository or served to third parties, so share-alike doesn't bite; add attribution text wherever OFF-sourced values are shown, matching what a "data source" label already implies once source is visible per Decision 2.
- **[Risk]** OFF full-export download size/format may change over time (JSONL vs CSV), unlike USDA's stable frozen SR Legacy zip. → **Mitigation**: `off.Fetch`/import mirrors `usda.Fetch`'s "accept a URL or a local path" flexibility, so a broken auto-download can be worked around by fetching the dump manually and importing from a local path.

## Migration Plan

No database migration: `off_code` is a new nullable column with no default data to backfill, and GORM's auto-migration (already used for the other food-logging tables) adds it on next startup. Deploy order: land the code (new column present but unused until an OFF database file exists), then run `import-off` once as an operator to populate `HCW_OFF_DB_PATH`. Until that import runs, `off.Open` returns `ErrNoDatabase` the same way `usda.Open` does for a missing file, and candidate resolution degrades to USDA-only — the existing behavior, unchanged. No rollback beyond reverting the code; the new column simply goes unused again.
