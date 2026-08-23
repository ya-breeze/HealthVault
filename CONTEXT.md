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
A per-screen, non-persisted view toggle (recognition/confirmation and Custom Food catalog — the two screens that show individual food names; the food history list shows only meal names and links into the former) that reveals a Food Item's Canonical Name alongside its Display Name. Strictly a *view* toggle: it changes what a screen shows and never what is stored or sent.
_Avoid_: Technical view, debug mode. Also do not shorten it to "Expert" — reanalysis already has an unrelated **Expert** authoring mode (see below), and on the review screen both are visible at once.

**Expert (reanalysis mode)**:
The reanalysis correction interface's second authoring mode, where a user lists the meal's components explicitly instead of writing a free-text `Hint`. Unrelated to Expert Mode above — it is an *input* mode that changes what gets analyzed, not a view toggle. The two share a word and a screen, so always qualify which one is meant: "Expert Mode" for the Canonical Name toggle, "Expert reanalysis" or "the Expert tab" for this one. The Expert Mode checkbox's visible label spells out its effect ("show English translation") for the same reason.
_Avoid_: Expert mode (lowercase "mode" reads as the toggle), component mode

### Dashboard

**Dashboard Card**:
An individual, independently showable/hideable and reorderable unit on the main dashboard — a Vital Card or a Food Card. Distinct from the dashboard's other sections (the needs-attention banner, Log Food row, More Data row), which are fixed JSX blocks, not Cards, and are not user-configurable.
_Avoid_: Widget, panel, section (reserve "section" for the fixed non-Card blocks)

**Vital Card**:
A Dashboard Card showing one vital metric's current value, 7-day sparkline, and trend arrow.
_Avoid_: Metric card, stat card

**Food Card**:
A Dashboard Card summarizing food-logging data — e.g. today's intake against a Nutrition Target, or a Healthiness Label.
_Avoid_: Nutrition widget, food widget

**Presence**:
Whether the resolved user has ever recorded at least one row of a given data type, computed over all time — the `GET /api/data-types/presence` signal used to hide a type everywhere on the dashboard (vitals grid and More Data) when the user has no data for it at all. Distinct from a Dashboard Card's `hidden` flag (a user preference, only meaningful for types that do have presence) and from the vitals grid's 7-day recency window (a metric with presence but no data in the last 7 days still renders its card, just with the "no data" sparkline placeholder).
_Avoid_: "Has data" (ambiguous with the recency window), "visible" (conflates with the `hidden` preference)

### Nutrition targets

**Goal Weight**:
The user's target body weight, stored as its own metric (latest-record-wins), distinct from a measured `Weight` reading. Read as the weight input to Nutrition Target calculations, not the user's current measured weight.
_Avoid_: Target weight, ideal weight (informal use only)

**Nutrition Target**:
A user's daily calorie/protein/carb/fat goal. Calories come from the Mifflin-St Jeor formula using the user's measured weight, height, age, sex, and activity level; protein is a rate applied to Goal Weight instead, since protein is sized to the body being worked toward, not the current one. What food intake is compared against on Food Cards.
_Avoid_: Goal (ambiguous with Goal Weight), macro goal

**Healthiness Label**:
A qualitative (Good / Fair / Needs attention), not numeric, assessment of how nutritious a user's food logging has been over a rolling window — computed by a deterministic heuristic over already-logged macros, not an LLM judgment.
_Avoid_: Health score, nutrition score
