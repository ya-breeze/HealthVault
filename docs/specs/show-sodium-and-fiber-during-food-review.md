# Show sodium and fiber during food review

## Why

HealthVault stores sodium and dietary fiber for every recognized or manually entered Food Item, but the food review row shows only calories, protein, carbohydrates, and fat. A user therefore cannot see the two values that affect nutrition guidance while checking a meal. The existing manual correction panel contains sodium and fiber fields, but it opens on food search and initializes every nutrient to zero, so correcting either value risks replacing the other recognized values with zeros.

The review step is the last point before a photo- or description-derived meal is confirmed. Show the stored values there and let the user open a prefilled nutrient editor directly.

## How

Extend each review item with a second localized nutrient line for sodium and dietary fiber. Keep sodium in elemental-sodium grams because that is the value stored by the API and evaluated by the Healthiness Label; do not relabel it as salt or silently convert units.

Add a localized “Edit nutrients” action for resolved and estimated items. It opens the existing manual nutrient form directly, prefilled with the item name and all seven current nutrient values. Saving continues through the existing manual item PATCH, so the correction becomes the authoritative manual profile and the meal totals refresh through the existing serialized mutation path. Keep the current search/rebind action available and leave unresolved items on their existing resolution flow.

The structured manual-entry form already exposes sodium and fiber before save, so this change does not duplicate another editor there. It does not change database fields, API shapes, nutrition thresholds, chat behavior, or production infrastructure. No ADR is needed because the change reuses the existing editing and persistence boundary.

## Validation Commands

```bash
make lint
make test
BASE_URL=http://192.168.1.54:8892 make test-e2e E2E_ARGS="tests/food.spec.ts --retries=0"
```

### Task 1: Expose the stored values

- [x] Add localized Sodium and Fiber labels to every resolved or estimated Food Item row on the review screen.
- [x] Keep the values visible for both pending-review and confirmed meals without changing meal or item totals.
- [x] Mark completed.

### Task 2: Open a safe prefilled correction form

- [x] Add an Edit nutrients action that opens ItemResolver in manual mode.
- [x] Initialize the manual form with the item name and all seven current nutrient values rather than zeros.
- [x] Preserve the existing search/rebind, reusable-food, serialized-update, and success/error behavior.
- [x] Mark completed.

### Task 3: Cover the review workflow

- [x] Add deterministic Playwright coverage that sees sodium and fiber before editing.
- [x] Assert the editor opens in manual mode with every nutrient prefilled and sends the corrected sodium and fiber together with the unchanged values.
- [x] Assert the returned meal immediately updates the displayed nutrient values.
- [x] Mark completed.

### Task 4: Validate and review

- [ ] Run the validation commands against the feature branch and deployed WIP stack.
- [ ] Run the Review Gate and resolve every valid finding.
- [ ] Confirm that no task box remains unticked.
- [ ] Mark completed.
