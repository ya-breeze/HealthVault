# HealthVault

A personal health-tracking app. Its food-logging capability lets a user photograph a meal, has an AI recognize what's in it, and tracks the resulting nutrition over time.

## Language

### Food logging

**Food Meal**:
A single photographed/logged eating occasion, made up of one or more Food Items.
_Avoid_: Meal photo, entry (too generic — could mean any logged record)

**Food Item**:
One recognized food or ingredient occurrence within a Food Meal.
_Avoid_: Ingredient (reserve for informal use only — a Food Item can be a whole dish, not just a component)

**Custom Food**:
A user's personal, reusable catalog entry for a food — a name plus its macros — created once (typically from a recognition) and then reused across future Food Items instead of re-recognizing or re-matching against a reference database.
_Avoid_: Recipe, saved food

### Localization

**Display Language**:
The per-user setting that controls which language a user sees the interface in, and which language the AI is asked to produce food/ingredient names in when recognizing a Food Item for that user.
_Avoid_: Locale, UI language

**Display Name**:
A Food Item's or Custom Food's name in the user's Display Language — what's shown to the user normally.
_Avoid_: Name (ambiguous once a Canonical Name also exists), localized name

**Canonical Name**:
A Food Item's English name, produced by the AI alongside the Display Name in the same recognition call, for Expert Mode to show regardless of Display Language.
_Avoid_: English name, technical name

**Expert Mode**:
A per-screen, non-persisted view toggle (recognition/confirmation, food history, Custom Food catalog) that reveals a Food Item's Canonical Name alongside its Display Name.
_Avoid_: Technical view, debug mode
