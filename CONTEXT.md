# HealthVault

A personal health-tracking app. Its food-logging capability lets a user photograph a meal, has an AI recognize what's in it, and tracks the resulting nutrition over time.

## Language

### Food logging

**Food Meal**:
A single photographed/logged eating occasion, made up of one or more Food Items.
_Avoid_: Meal photo, entry (too generic — could mean any logged record)

**Meal Description**:
The user's own free-text account of a meal, written when logging it through the description-first manual entry path instead of a photo — the input to `vision.Client.Describe`. Persisted on the Food Meal (empty for a photographed or structured-manual entry) so a failed or retried analysis can replay it. Distinct from the photo `Hint`: a Hint only decorates a single `Recognize` call for a photographed meal and is never stored, while a Meal Description *is* the meal's only input and has to survive across a retry.
_Avoid_: Hint, note (too generic, and collides with the unrelated photo Hint)

**Food Item**:
One recognized food or ingredient occurrence within a Food Meal.
_Avoid_: Ingredient (reserve for informal use only — a Food Item can be a whole dish, not just a component)

**Custom Food**:
A user's personal, reusable catalog entry for a food — a name plus its macros — created once (typically from a recognition) and then reused across future Food Items instead of re-recognizing or re-matching against a reference database.
_Avoid_: Recipe, saved food

**Eating Occasion**:
A group of a Logged Day's Food Meals collapsed by proximity — a new occasion starts whenever the gap to the previous Food Meal's `LoggedAt` exceeds 10 minutes, otherwise it merges into the current one. Exists so a single sitting logged as 2-3 follow-up photos isn't double-counted toward Day Completeness.
_Avoid_: Meal count, session (too generic)

**Logged Day**:
The calendar date (`YYYY-MM-DD`) a Food Meal's `LoggedAt` falls on, computed in the user's stored `timezone` setting (absent/invalid → UTC) — not the browser's local zone. "Today" in the user's zone is always excluded from Day Completeness.

**Usual Meals Per Day**:
The per-user `usual_meals_per_day` setting (positive integer, default 3): the Eating Occasion count a Logged Day must reach to be classified automatically Complete. Read fresh on every Day Completeness computation, not snapshotted, so changing it re-evaluates past days too.
_Avoid_: Meal target, threshold (informal use only)

**Day Completeness**:
One of four states computed per completed Logged Day from its Eating Occasion count against Usual Meals Per Day and whether the user has confirmed it: **Incomplete** (0 occasions), **Unconfirmed** (below threshold, unconfirmed), **Confirmed Complete** (below threshold, user-confirmed), **Complete** (at/above threshold). A user assertion gated by a heuristic that only decides *when to ask* — never a judgment of the day's nutrition, and never computed for today.
_Avoid_: Completion status, logging status

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
An individual, independently showable/hideable and reorderable unit on the main dashboard — a Vital Card or a Food Card. Distinct from the dashboard's other sections (the needs-attention banner, Log Food row, More Data row), which are fixed JSX blocks, not Cards. The needs-attention banner and Log Food row are not user-configurable; the More Data row is the exception — it has its own persisted, user-configurable hide/show preference (`more_data_hidden`), independent of the per-Card `hidden` flag, though it remains a single fixed block rather than a reorderable Card.
_Avoid_: Widget, panel, section (reserve "section" for the fixed non-Card blocks)

**Vital Card**:
A Dashboard Card showing one vital metric's current value, 7-day sparkline, and trend arrow.
_Avoid_: Metric card, stat card

**Food Card**:
A Dashboard Card summarizing food-logging data — e.g. today's intake against a Nutrition Target, a Healthiness Label, or the Logging Gap Card.
_Avoid_: Nutrition widget, food widget

**Presence**:
Whether the resolved user has ever recorded at least one row of a given data type, computed over all time — the `GET /api/data-types/presence` signal used to hide a type everywhere on the dashboard (vitals grid and More Data) when the user has no data for it at all. Distinct from a Dashboard Card's `hidden` flag (a user preference, only meaningful for types that do have presence) and from the vitals grid's 7-day recency window (a metric with presence but no data in the last 7 days still renders its card, just with the "no data" sparkline placeholder).
_Avoid_: "Has data" (ambiguous with the recency window), "visible" (conflates with the `hidden` preference)

### Nutrition targets

**Goal Weight**:
The user's target body weight, stored as its own metric (latest-record-wins), distinct from a measured `Weight` reading. Read as the weight input to Nutrition Target calculations, not the user's current measured weight.
_Avoid_: Target weight, ideal weight (informal use only)

**Nutrition Target**:
A user's daily calorie/protein/carb/fat goal, computed on read by `GET
/api/users/me/nutrition-target` from the user's measured weight/height, Goal Weight, and two new
profile fields (`birthdate`, `sex`) plus Activity Level — not stored. Calories come from the
Mifflin-St Jeor formula (measured weight/height/age/sex) times the Activity Level multiplier;
protein is a g/kg rate applied to Goal Weight instead, since protein is sized to the body being
worked toward, not the current one; carbs/fat split whatever calorie budget remains. Requires a
goal weight unconditionally — there is no partial target, and no fallback to measured weight when
no goal is set (see ADR-003). What food intake is compared against on Food Cards.
_Avoid_: Goal (ambiguous with Goal Weight), macro goal

**Daily Summary**:
The single self-only, computed-on-read aggregate exposed at `GET /api/summary/today`: today's
consumed calories/protein/carbs/fat (confirmed Food Meals only), a raw meal count and
last-logged timestamp (any status), the caller's Display Language, and an embedded Nutrition
Target (or its unavailable-reason code — never a 422 here, since an unavailable target is a
normal state on this endpoint). Exists so a thin client gets one call instead of composing
Nutrition Target and food history itself.
_Avoid_: Today's summary (informal), dashboard summary (distinct from the dashboard's own Food
Card rendering, which is a separate read path)

**Trend Weight**:
The exponential moving average (alpha 0.25) of the outlier-filtered, day-bucketed `weight` series — the same EMA function and alpha the weight chart's own trend line (see Trend Projection) already uses, so the term means the same underlying computation everywhere the app shows it. The Logging Gap Card fits its slope over exactly this series' last 28 calendar days.
_Avoid_: Trend line (reserve for the weight chart's rendered line specifically), smoothed weight

**Implied Intake**:
The daily calorie intake the Logging Gap Card's weight trend implies the caller actually ate: `Nutrition Target calories + (Trend Weight slope kg/day * 7700)`. Derived from energy balance (a weight trend implies a net calorie balance), not measured or logged — distinct from Mean Logged Intake, the average of the caller's actual food-log totals over the same window.
_Avoid_: Estimated intake (ambiguous with Mean Logged Intake)

**Logging Gap**:
`Implied Intake - Mean Logged Intake`, rendered on the Logging Gap Card as a kcal/day range with an uncertainty interval, never a bare point estimate — the calorie amount the weight trend implies that isn't showing up in the food log. Deliberately not a TDEE, and does not touch or replace ADR-006's activity multiplier; see ADR-010.
_Avoid_: TDEE, adaptive TDEE (the original framing this feature replaced; see ADR-010), total calories (an existing, unrelated table name)

**Activity Level**:
One of 5 standard tiers (Sedentary / Lightly active / Moderately active / Very active / Extra
active, multipliers 1.2/1.375/1.55/1.725/1.9) feeding the Nutrition Target's calorie calculation.
Inferred by default from a trailing 28-day average of the user's `steps` records (excluding
zero-record days anywhere in the window and trimming a sub-500-step run only from the trailing
edge), or taken verbatim from an optional `activity_override` profile field when set — the two
signals are never blended. Fewer than 7 valid trailing days with no override set makes the
Nutrition Target uncomputable (`insufficient_activity_data`). See ADR-006.
_Avoid_: Activity multiplier alone (that's the numeric output, not the tier), exercise level

**Healthiness Label**:
A qualitative (Good / Fair / Needs attention), not numeric, assessment of how nutritious a user's food logging has been over a rolling window — computed by a deterministic heuristic over already-logged macros, not an LLM judgment.
_Avoid_: Health score, nutrition score

### Weight chart

**Bucket Start**:
The local calendar date (or, for a month bucket, the first of the local calendar month) a bucketed `GET /api/data/{type}?bucket=day|month` row covers, resolved in the user's stored `timezone` setting (absent/invalid → UTC — the same fallback Logged Day uses) and serialized as the `bucket_start` field, a `YYYY-MM-DDT00:00:00Z` string naming that calendar date at UTC midnight, not the instant local midnight occurred.
_Avoid_: bucket date, bucket timestamp

**Manual Record**:
A metric-type record (`weight`, `height`, or `weight_goal` only — the write allowlist) created directly by the user through the Add-record form, via `POST /api/data/{type}`, rather than by ingestion (CSV import, MCP tool call, food-photo recognition). The distinction matters only at write time; a Manual Record reads back identically to an ingested one.
_Avoid_: Manual entry (ambiguous with food logging's manual entry mode)

**BMI Band**:
One of the 4 WHO BMI category ranges (Underweight, Normal, Overweight, Obese) rendered as a horizontal `ReferenceArea` on the weight chart, converted from BMI thresholds (18.5, 25, 30) to kilograms using the user's latest `height` record. Absent entirely when no `height` record exists.
_Avoid_: BMI zone, weight range

**Trend Projection**:
A dashed line extrapolating the weight chart's existing EMA trend line forward, via least-squares regression over the last 30 calendar days of EMA values, to the calendar date it's projected to cross Goal Weight. Rendered only at Month/Year zoom; the plain-language ETA text it produces ("on track", "not on track", "already reached", "not enough data") renders at every zoom level and only appears at all when a Goal Weight is set.
_Avoid_: Forecast, prediction line

### Authentication

**Access Assertion**:
The signed JWT Cloudflare Access attaches as the `Cf-Access-Jwt-Assertion` header once its policy has approved a Google sign-in. Verified by `backend/pkg/cfaccess` against Cloudflare's own published JWKS (RS256, issuer/audience pinned, `exp`/`nbf` checked) — never trusted on the strength of the header's mere presence, since the backend is also reachable directly on the LAN, bypassing Cloudflare, where any header is attacker-set. See ADR-012.
_Avoid_: Access token (ambiguous with HealthVault's own `kin_access` JWT), Cf-Access header

**Access Identity Map**:
The `HCW_CF_ACCESS_EMAIL_MAP` setting (`email:username`, comma-separated, shaped like `HCW_SEED_USERS`) that authorizes a verified Access Assertion's email to sign in as a specific HealthVault user. An email that verifies but is absent from the map is refused (403), rather than auto-provisioning an account — widening the Cloudflare Access policy, a different system, never silently creates a HealthVault user. See ADR-012.
_Avoid_: Email map alone (ambiguous outside this context), user mapping
