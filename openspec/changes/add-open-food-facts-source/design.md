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
- Sending the photo into the candidate-*selection* call. Considered as a fix for brand ambiguity (see Decisions) and rejected in favor of extracting brand once during *recognition*, which already sees the photo — touching `Select`'s signature would affect USDA and custom-food selection too, for no benefit to those paths.
- Barcode-scan-driven food entry. `Index.ByCode` is built because binding a chosen candidate needs it, but no scan UI is added here.
- NutriDatabaze.cz (Czech generic-food database) — needs account registration and a license read a human has to do; stays a `todo.md` backlog note.
- Any change to how `CustomFood` precedence works (case-insensitive exact name match still wins over both USDA and OFF, unchanged).

## Decisions

### Third parallel nullable column, not a generalized Source+ExternalID

**Decision**: Add `FoodItem.OffCode *string` and `GroundTruthItem.OffCode *string` alongside the existing `FdcID`/`CustomFoodID`, rather than collapsing all three into a single `ReferenceSource string` + `ReferenceID string` pair.

**Why**: The codebase already carries two parallel nullable reference columns with pairwise exclusivity checks at each call site. A third column extends that exact pattern — one more nullable field, one more branch in each "at most one set" check — with zero migration of existing data. A generalized `Source+ExternalID` column would require backfilling every existing `FdcID` row with `source="usda"`, rewriting every lookup call site to branch on a string discriminator instead of a nil check, and gains nothing functionally: the discriminator is already implicit in which field is non-nil. This was raised and explicitly rejected during design review as unnecessary churn for the value it returns.

**Alternative considered**: `Source + ExternalID string` pair (what `todo.md`'s original research assumed would be needed). Rejected as disproportionate to a two-source (soon possibly three-source) system where "which column is non-nil" already answers "which source."

### Brand extracted at recognition time gates whether OFF is queried at all

**Decision**: `vision.Item` gains `Brand string` (empty when no label/brand is legible in the photo), extracted by the same `Recognize` call that already extracts `Preparation`/`State` — not by giving the later `Select` call access to the photo.

**Why**: `Select` is a text-only model call (`Select(ctx, itemCandidates []ItemCandidates)` — no image parameter) shared across USDA, custom-food, and now OFF candidates. For USDA, a generic name like "chicken breast" resolving to *some* SR Legacy chicken-breast entry is fine — different preparations of the same generic food have bounded macro variance. For OFF, a generic name like "yogurt" can match a dozen genuinely different branded products whose real macros vary far more than USDA's cooking-method variants, and the model has no way to tell which one was actually photographed. Auto-binding to an arbitrary branded product is not obviously better than USDA's generic estimate — it can be worse, while looking more precise. Extracting brand during `Recognize`, which already has the photo, gives a real disambiguating signal without touching `Select`'s signature (and therefore without adding photo cost to USDA/custom-food selection, which don't need it).

**Rejected alternative**: pass the photo into `Select` too. Would give the model the same information more directly, but widens `Select`'s contract for all three sources and adds a vision-call cost to every selection, not just OFF ones, for no benefit to USDA/custom-food matching.

**Rejected alternative**: a text heuristic on the recognized name ("specific enough" wording) to decide whether to query OFF at all. Rejected as unreliable — a heuristic over free-text model output has no real signal to key on, whereas brand extraction reuses a capability (structured extraction from the photo) the recognition call already has.

### OFF queried only when a brand was extracted, USDA as the fallback

**Decision**: Candidate resolution in `food_upload.go`, for an item with a non-empty `Brand`, builds the retrieval term from name+brand and calls `off.Search`; if that returns zero candidates, it falls back to `usda.Search`. For an item with no `Brand` extracted, USDA is queried directly and OFF is not queried at all.

**Why**: bm25 scores are corpus-relative — merging OFF and USDA hits into one ranked-by-score list would compare numbers that don't mean the same thing across two different FTS5 indexes, so a fallback rule is used instead of a merge. Gating on brand presence (rather than querying OFF unconditionally) is what makes that fallback rule safe: OFF only gets to supply the shortlist when there's an actual signal — the recognized brand — pointing at a real product, which is also the case that matters (the yogurt-protein example: a real CZ product's label beats a generic USDA estimate specifically because the model saw and reported the brand on the carton).

**Residual limitation**: brand extraction can still be wrong (misread label, OCR-adjacent error) or the extracted brand text may not match how OFF's `brands` field is written for that product, in which case OFF returns zero candidates and USDA is used — degrading safely rather than binding to a wrong product. What no longer happens is auto-binding to an arbitrary OFF product for a *brandless* generic query, which was the actual risk (raised in review) with the earlier zero-result-only trigger.

**Alternative considered**: always query both and interleave top-N from each with a visible source badge (considered during design review, not chosen — the user picked the simpler EU-first/US-fallback rule over interleaving both sources' results on every search).

### Country/completeness filter on OFF import, mirroring the USDA import's own scope limit

**Decision**: `import-off` filters the OFF export to products whose `countries_tags` include Czech Republic and/or Slovakia (extendable to EU broadly) and whose `nutriments` contain complete calories/protein/carbs/fat, before writing to the local SQLite+FTS5 index.

**Why**: The full OFF export is 3.5M+ globally-sourced branded products of uneven quality. Importing all of it would make the local index huge and mostly irrelevant to a Czech user, and would slow every search. This mirrors how `usda`'s importer only takes Foundation + SR Legacy (~8k curated rows) rather than the full FDC Branded Foods dataset. `off.Builder.Promote` reuses the same `MinExpectedRows`-style guard as `usda.Builder` so a truncated/bad filtered import never replaces a working database.

**Scope for v1**: `countries_tags` limited to Czech Republic and Slovakia only, not EU-wide. Narrowing the corpus is strictly the safer failure mode (fewer, higher-confidence matches) than widening it, so starting narrow and widening later — if broader EU coverage turns out to be useful — costs nothing but a re-import.

### Explicit per-field mapping and unit handling for OFF nutriments, not a generic pass-through

**Decision**: The import mapping reads `nutriments.energy-kcal_100g` for calories (never `energy_100g` or `energy-kj_100g` directly), and reads `nutriments.sodium_100g` for sodium when present, falling back to `nutriments.salt_100g / 2.5` when only salt is present. A product missing calories or missing both sodium and salt is treated as incomplete and excluded by the existing completeness filter, the same as missing protein/carbs/fat.

**Why**: OFF's schema carries multiple representations of the same nutrient for historical/regional-labeling reasons — `energy-kcal_100g` and `energy-kj_100g` both exist, and EU labels moved from sodium to salt in 2013 (salt = 2.5 × sodium), so many EU products only populate `salt_100g`. A pass-through mapping that reads whichever field happens to be present without normalizing units would silently import wrong values (e.g. a kJ figure read into a kcal field is off by roughly 4×) rather than failing loudly — worse than rejecting the row, since a wrong-but-plausible-looking number doesn't trip the row-count sanity check the way a filtered-out row does.

**Alternative considered**: import whatever `energy_100g`/`sodium_100g` fields are present unconditionally. Rejected — `energy_100g`'s unit is inconsistent across older vs. newer OFF entries, which is exactly the kind of silent-corruption risk the explicit-field mapping avoids.

### `off` package structure mirrors `usda` package structure

**Decision**: `backend/pkg/off` reimplements `Index.Search(term, limit)`, `Index.ByCode(code string)`, `Builder`/`Add`/`Promote`/`Discard`, and the same `sanitizeFTSQuery` OR-join approach, as a parallel package rather than a shared generic abstraction over both.

**Why**: The two sources have genuinely different schemas (OFF has `code`/`brands`, no `data_type`; nutrient completeness and filtering rules differ). Two small, independently-buildable packages that happen to look similar are easier to reason about and modify independently than one generic package parameterized over both, especially since the import/filter logic (the part most likely to need tuning per source) is exactly where they differ most. `usda.go`'s existing doc comments about bm25 length bias, bm25 corpus-relativity, and bare punctuation causing FTS5 syntax errors apply unchanged to the new package's FTS5 usage and don't need to be relearned.

## Risks / Trade-offs

- **[Risk]** Brand extraction can misread a label or use wording that doesn't match how OFF's `brands` field is written, so a query that should have matched returns zero and falls back to USDA — a missed opportunity, not a wrong answer, since the brand-gate means OFF never auto-binds without a real signal in the first place. → **Mitigation**: none needed beyond the brand-gating decision itself; this degrades to today's USDA-only behavior rather than to a wrong bind.
- **[Risk]** `food_manual.go`'s `CreateManualMeal` currently has no exclusivity check between `FdcID` and `CustomFoodID` at all — it resolves via `FdcID` first if both are set, but persists both fields regardless. Adding `OffCode` without fixing this compounds a pre-existing gap rather than just adding to one already enforced. → **Mitigation**: task 4.3 adds this check as new, not "extends" existing coverage — see tasks.md.
- **[Risk]** OFF's ODbL 1.0 license requires attribution, and share-alike terms apply if the *built* filtered database were itself redistributed publicly. → **Mitigation**: the built SQLite index is operator-local data (like the USDA one), not committed to the repository or served to third parties, so share-alike doesn't bite; add attribution text wherever OFF-sourced values are shown, alongside the source badge already needed for the brand-gated fallback decision above.
- **[Risk]** OFF full-export download size/format may change over time (JSONL vs CSV), unlike USDA's stable frozen SR Legacy zip. → **Mitigation**: `off.Fetch`/import mirrors `usda.Fetch`'s "accept a URL or a local path" flexibility, so a broken auto-download can be worked around by fetching the dump manually and importing from a local path.

## Migration Plan

No database migration: `off_code` and `brand` are new nullable columns with no default data to backfill, and GORM's auto-migration (already used for the other food-logging tables) adds them on next startup. Deploy order: land the code (new columns present but unused until an OFF database file exists), then run `import-off` once as an operator to populate `HCW_OFF_DB_PATH`.

**Requires a backend restart to take effect.** `off.Open` is called once during server construction, exactly like `usda.Open` (`server.go:52`) — the resulting handle is held for the life of the process. Running `import-off` while the server is already up does not make the running process pick up the new database: the first import needs a restart to activate it at all, and any later reimport needs a restart too, since the server's existing SQLite connection(s) may keep reading through the pre-rename file rather than the atomically-renamed replacement. This is not new: `usda.Open`/`import-usda` already have the identical characteristic today, undocumented until now. Until the first restart-after-import, `off.Open` returns `ErrNoDatabase` the same way `usda.Open` does for a missing file, and candidate resolution degrades to USDA-only — the existing behavior, unchanged. No rollback beyond reverting the code; the new columns simply go unused again.
