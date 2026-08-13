## Why

Photo food recognition currently decomposes nearly every meal into its individual ingredients (e.g. "beans 30g, corn 30g, tomato 20g") because the Recognize prompt asks the vision model to identify "each distinct food item." For composite dishes the user cannot decompose from memory anyway (a bought sauce, a restaurant mix), this produces noisy, over-granular logs and prevents the existing per-user custom-food reuse mechanism from ever matching, since a differently-decomposed item list rarely repeats item-for-item. The user should be able to log "Mexican vegetable mix 200g" as one line, enter its macros once (e.g. from a package label), and have that reused automatically the next time a similar-looking dish is photographed — without losing the option to fall back to fine-grained ingredients when a dish visibly has separate components.

## What Changes

- Recognize's prompt and schema change so the vision model defaults to naming a whole composite dish as one item, decomposing into multiple items only when it visually observes clearly separate components (e.g. a plate with a salad pile, a protein, and a side). This is a model judgment call, not a user-facing setting — no new toggle is introduced.
- Custom-food candidate retrieval, currently a case-insensitive exact-name match only, is extended to also always offer the user's own custom foods ranked by usage frequency (weighted higher) and recency as additional candidates to the existing candidate-selection model call — so a differently-worded recognized item name (e.g. "Mexican veggie mix" vs. a saved "Mexican vegetable mix") can still be matched to previously entered macros. The exact-name short-circuit is unchanged; this only widens what else is offered when it doesn't fire.
- A new macro-source priority is established: (1) a matched custom food (existing or newly-widened match) takes precedence over (2) a USDA/Open Food Facts reference match, which takes precedence over (3) a new fallback — if candidate selection finds no match at all, the same Recognize call also estimates the item's macros directly from the photo rather than leaving it with no usable macros.
- A new `macro_source` value, `estimated`, is introduced for this fallback, distinct from `reference` (bound to a real database/custom-food record) and `manual` (user-entered). It is treated as a usable macro source for meal aggregation (like `reference` and `manual`) and is surfaced distinctly in the confirm-review UI as an unverified AI estimate rather than a database fact.
- No new entities, no new user-facing mode/toggle for initial upload. The existing Expert reanalysis correction path (`POST /api/food/meals/{id}/reanalyze` with `components`) is unchanged and remains a post-hoc correction path only.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `food-photo-recognition`: Recognize defaults to composite-dish-level item naming instead of ingredient-level decomposition; adds a macro-estimate fallback within the same Recognize call when candidate selection finds no match, producing `macro_source = estimated`.
- `usda-nutrition-database`: "Match Selection and Explicit Non-Match" is extended so custom-food candidates offered to selection include the user's frequency/recency-ranked custom foods, not only an exact case-insensitive name match.
- `food-nutrition-logging`: the macro-aggregation requirement is extended to treat `macro_source = estimated` as a usable macro source (alongside `reference` and `manual`) when summing a confirmed meal's totals.

## Impact

- Backend: `backend/pkg/vision/openai.go` (Recognize prompt/schema, new estimate output), `backend/pkg/vision/vision.go` (`Item` struct gains an optional estimated profile), `backend/pkg/server/food_upload.go` (`retrieveCandidates` gains frequency/recency-ranked custom-food candidates; item resolution honors the new priority and `estimated` source), `backend/pkg/database/models_food.go` (`MacroSource` gains `estimated`).
- Frontend: meal confirm-review UI gains a distinct visual treatment for `estimated` items (e.g. `frontend/components/food/...`), separate from the existing `reference`/`manual`/`none` treatment.
- No database migration for new tables; `macro_source` is a string column with existing values, so no schema migration is required for the new `estimated` value itself. Frequency/recency ranking is computed by querying existing `FoodItem`/`FoodMeal` rows, not by adding new columns.
