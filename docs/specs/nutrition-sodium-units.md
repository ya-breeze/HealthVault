# Nutrition recognition sodium units

## Why

The food-recognition model can return a sodium estimate in milligrams even though HealthVault stores `sodium_per_100g` in grams. A dry salami estimate of `1500` for a 40 g portion can therefore be interpreted as 1500 grams per 100 g and produce an impossible portion total. The photo and text recognition prompts need an unambiguous field-level unit and per-100 g example.

## How

State in one shared instruction used by photo and text recognition that every estimated profile value is per 100 g, sodium is expressed in grams per 100 g, and a milligram label must be converted and normalized to that basis. Give the concrete example that a label showing 1500 mg sodium per 100 g maps to `sodium_per_100g: 1.5`; a 40 g portion contains about 0.6 g total sodium, while its per-100 g value remains 1.5. Add a schema description to the sodium field that repeats its unit and basis. Do not add conversion guesses or change model routing.

## Validation Commands

- `make test-backend`
- `make lint`
- `make test-e2e`

### Task 1: Make the sodium units explicit
- [ ] Add a shared sodium unit directive to both recognition prompt paths, with the 1500 mg per 100 g and 40 g portion example.
- [ ] Describe `sodium_per_100g` in the structured output schema as grams per 100 g.
- [ ] Add regression coverage for both prompt paths and the schema field description.
- [ ] Run the validation commands and validate a synthetic recognition request against hcw-wip using the real configured provider.
- [ ] Mark completed
