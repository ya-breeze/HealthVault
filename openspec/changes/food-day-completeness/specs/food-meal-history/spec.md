## MODIFIED Requirements

### Requirement: Meal History Page
The frontend SHALL provide a meal history page reachable from the dashboard, listing the caller's meals from `GET /api/food/meals` grouped into sections by Logged Day (the meal's `logged_at` converted into the user's stored `timezone` setting per the `food-day-completeness` capability, defaulting to `UTC` when unset — not the browser's local timezone), most-recent day first. Each day section SHALL show a header identifying that calendar day, followed by that day's meals in the existing per-meal format (name, logged date/time, status, and calories, blank for a meal whose status is not `confirmed`), each linking to that meal's existing review page (`/food/review/?meal=<id>`). Each day section SHALL also show a daily total — summed calories, protein, carbs, and fat — computed only over that day's `confirmed` meals; a day with no confirmed meals SHALL show a total of zero rather than omitting the total line. The page SHALL offer a "load older" action that re-queries with both `before` and `before_id` set from the oldest meal currently shown, so meals beyond the first page remain reachable via the lossless keyset cursor rather than the plain timestamp filter; meals returned by "load older" SHALL be merged into the correct existing day section or form new day sections as needed, keeping every day's total accurate as more meals load.

For every rendered day section other than the caller's current Logged Day, the page SHALL fetch and display that day's completeness state from `GET /api/food/completeness` (see `food-day-completeness`) and render, per state: nothing extra for `complete`; a badge indicating the day is confirmed complete, plus an affordance to retract that confirmation, for `confirmed_complete`; and an affordance to confirm the day complete for `unconfirmed`. A day section for the caller's current Logged Day SHALL render no completeness badge or control, since the range endpoint never returns a state for it. This is the only surface in the product that shows a completeness badge or offers the confirm/retract action — no dashboard banner, notification, or post-meal-log prompt exists for it.

#### Scenario: History page is reachable from the dashboard
- **WHEN** an authenticated user is on the dashboard
- **THEN** a link to the meal history page is visible

#### Scenario: History row links to the review page
- **WHEN** a user clicks a meal row in the history list
- **THEN** the system navigates to `/food/review/?meal=<that meal's id>`

#### Scenario: Unconfirmed meal shows no calorie total
- **WHEN** the history list includes a meal whose status is `pending_review`, `pending_clarification`, `processing`, or `failed`
- **THEN** that row shows no calorie total, since the meal has none computed yet

#### Scenario: Loading older meals
- **WHEN** the user activates "load older" at the bottom of the list
- **THEN** the system fetches the next page using both `before` and `before_id` and appends it, without losing the meals already shown

#### Scenario: Meals are grouped by local calendar day
- **GIVEN** the caller has meals whose `logged_at` values fall on two different Logged Days once converted into their stored `timezone` setting (even if those meals fall on the same calendar date in the browser's own local timezone)
- **WHEN** they view the history page
- **THEN** the meals appear under two separate day headers reflecting the stored-timezone boundary, most recent day first, each containing only that day's meals

#### Scenario: No timezone set falls back to UTC grouping
- **GIVEN** the caller has no `timezone` key in their settings
- **WHEN** they view the history page
- **THEN** meals are grouped by their UTC calendar date, matching this page's behavior before the `food-day-completeness` change

#### Scenario: Daily total sums only confirmed meals
- **GIVEN** a day has one `confirmed` meal and one `pending_review` meal
- **WHEN** the user views that day's section
- **THEN** the displayed daily total (calories, protein, carbs, fat) reflects only the confirmed meal, and the pending meal still appears in the list below it with no calorie total on its own row

#### Scenario: Day with no confirmed meals shows a zero total
- **GIVEN** a day has only meals with status other than `confirmed`
- **WHEN** the user views that day's section
- **THEN** the daily total line is shown with all values at zero, not omitted

#### Scenario: Loading older meals updates day groupings correctly
- **GIVEN** the currently loaded meals end mid-way through a Logged Day
- **WHEN** the user activates "load older" and the next page includes more meals from that same day plus meals from an earlier day
- **THEN** the earlier-loaded day's section gains the newly loaded meals (and its total updates to include them), and a new day section appears for the earlier day

#### Scenario: Complete day shows no control
- **GIVEN** a day section's completeness state is `complete`
- **WHEN** the user views that section
- **THEN** no badge or confirmation control is shown for it

#### Scenario: Unconfirmed day shows a confirm affordance
- **GIVEN** a day section's completeness state is `unconfirmed`
- **WHEN** the user views that section
- **THEN** a control to confirm the day complete is shown, and activating it calls the day-confirmation API and updates the section to show `confirmed_complete` on success

#### Scenario: Confirmed day shows a retract affordance
- **GIVEN** a day section's completeness state is `confirmed_complete`
- **WHEN** the user views that section
- **THEN** a badge and a retract control are shown, and activating retract calls the day-confirmation API and updates the section to show its state after retraction

#### Scenario: Today's section shows no completeness control
- **GIVEN** a day section is the caller's current Logged Day
- **WHEN** the user views that section
- **THEN** no completeness badge or control is shown, regardless of how many meals are logged so far that day
