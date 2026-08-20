## ADDED Requirements

### Requirement: Per-User Display Language Setting

The system SHALL store the authenticated user's Display Language as a `display_language` key in
their existing `UserSettings` JSON object (`GET`/`PUT /api/users/me/settings`), requiring no new
storage or endpoint. An absent `display_language` key SHALL be treated as English (`en`) — the
existing default before this change, so no existing user's behavior changes until they explicitly
set a different Display Language.

Because `display_language` lives in an opaque settings object that `PUT` stores verbatim, its value
SHALL be interpreted rather than assumed to be one of the languages the UI ships. Interpretation
SHALL compare only the primary subtag of the tag — the part before the first `-` or `_` —
case-insensitively and ignoring surrounding whitespace, and the *same* interpretation SHALL govern
both UI rendering and recognition requests. A value that is not a well-formed language tag SHALL be
treated as unset, as SHALL a value whose primary subtag names a language the interface ships no
translations for — such a value SHALL NOT reach recognition either. Interpreting the one stored
value differently in the two places is specifically prohibited: it would let a user's interface
render in English while recognition is asked for another language and the English-vocabulary
reference databases are suppressed for them, a divergence nothing in the UI would reveal.

Because a well-formed tag can be stored by any caller of `PUT /api/users/me/settings`, and the
switcher offers only the languages the interface ships, restricting recognition to those same
languages is what keeps that divergence unreachable rather than merely unlikely — there is
otherwise no route back out of it through the UI, which would show "English" for a setting that is
not English.

#### Scenario: A language the interface does not ship is treated as unset

- **WHEN** a user's stored `display_language` names a language the interface ships no translations
  for, such as `de`
- **THEN** the system SHALL render the interface in English *and* request English Display Names
  from recognition, leaving reference-database matching enabled, rather than recognizing in that
  language behind an English interface

#### Scenario: A regional tag resolves the same way for the UI and for recognition

- **WHEN** a user's stored `display_language` is a regional tag such as `ru-RU` rather than the bare
  `ru` the switcher writes
- **THEN** the system SHALL render the interface in Russian *and* request Russian Display Names from
  recognition, rather than rendering English while recognizing in Russian

#### Scenario: Setting a Display Language

- **WHEN** an authenticated user sends `PUT /api/users/me/settings` with a body including
  `"display_language": "ru"`
- **THEN** the system stores it as part of their settings object, and subsequent reads of
  `GET /api/users/me/settings` include it

#### Scenario: Unset Display Language defaults to English

- **WHEN** an authenticated user's settings object has no `display_language` key
- **THEN** the system SHALL treat their Display Language as English for both UI rendering and
  recognition requests

### Requirement: UI Language Switcher

The frontend SHALL provide a control for the authenticated user to change their Display Language,
which writes the new value via `PUT /api/users/me/settings` and re-renders static UI strings in
the newly selected language without requiring a page reload.

#### Scenario: Switching Display Language updates the UI immediately

- **WHEN** a user selects a different Display Language from the switcher
- **THEN** the system saves the new setting and the interface's static strings SHALL reflect the
  new language without a full page reload

### Requirement: Localized Surfaces

The Display Language SHALL govern the dashboard — its vitals grid, the meal-attention link, the
log-food actions, and the secondary-metric links — in addition to the food recognition/confirmation
screen, the meal history, and the Custom Food catalog. Metric names SHALL be translated rather than
derived from their internal type identifiers, since deriving a label by substituting separators in
`basal_metabolic_rate` can only ever produce English.

The dashboard is named explicitly because it is the application's landing page: a user whose
Display Language is Russian otherwise meets an entirely English screen before reaching any surface
this capability translates, which makes the setting look inoperative.

A count rendered inside a sentence SHALL select its wording using the plural category the Display
Language defines for that number, not by testing whether the count equals one. English has two such
categories and Russian has four, so an `n === 1` test produces a wrong form for most Russian values
— including every number from 2 to 4, which are the common ones here.

Dates and times SHALL be formatted for the Display Language rather than for the browser's own
locale. The premise of this capability is that a user reads Russian while their device may be
configured in another language, so deferring to the browser leaves English weekdays and month names
on an otherwise Russian page. For English the browser's regional preference SHALL continue to be
honoured, since "English" alone does not determine whether a date reads as month-first or day-first
and the user's device is the better authority on that.

The following are deliberately out of scope and remain English regardless of Display Language: the
per-type data detail pages, the import screen, and the food entry/editing chrome enumerated in the
frontend dictionary's scope comment.

#### Scenario: The dashboard renders in the selected Display Language

- **WHEN** a user whose Display Language is Russian opens the dashboard
- **THEN** the vitals grid's metric names and empty states, the log-food actions, and the
  secondary-metric links SHALL be rendered in Russian

#### Scenario: A counted noun takes the right plural form

- **WHEN** the dashboard reports that some number of meals need attention to a user whose Display
  Language is Russian
- **THEN** the wording SHALL match the plural category Russian defines for that number, so that 1,
  3, and 5 each select their own form rather than only distinguishing 1 from everything else

#### Scenario: Dates follow the Display Language, not the browser

- **WHEN** a user whose Display Language is Russian views the meal history on a device configured
  for another locale
- **THEN** weekday names, month names, and timestamps SHALL be rendered in Russian

### Requirement: Display Language Passed to Recognition

The system SHALL include the authenticated user's current Display Language in every food-photo
recognition request (initial analysis, reanalysis, and clarification rounds), so the vision model
knows which language to produce the Display Name in for that call (see
`food-photo-recognition` "Food Recognition and Clarification Questions").

#### Scenario: A non-English Display Language is passed to recognition

- **WHEN** a user whose Display Language is `ru` uploads a food photo
- **THEN** the recognition request SHALL include `ru` as the target language for the Display Name
