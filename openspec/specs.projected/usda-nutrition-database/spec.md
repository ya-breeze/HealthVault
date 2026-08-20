<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See openspec/specs.projected.README.md for details. -->

# usda-nutrition-database Specification

## Purpose
TBD - created by archiving change photo-food-nutrition-logging. Update Purpose after archive.
## Requirements
### Requirement: Local USDA SQLite Storage and FTS5 Candidate Retrieval
The system SHALL maintain a local SQLite database, separate from the application database, containing USDA FoodData Central Foundation and SR Legacy foods with an FTS5 full-text index. A search SHALL return a ranked list of candidate foods with their per-100g macro profiles; it SHALL NOT bind an item to a food on its own.

#### Scenario: Full-text search returns ranked candidates
- **WHEN** searching for a food term such as "grilled chicken breast"
- **THEN** the system queries the FTS5 index and returns up to the configured number of ranked candidates, each with its FDC ID, description, and per-100g macro profile

#### Scenario: Search with no results
- **WHEN** a search term matches no indexed food
- **THEN** the system returns an empty candidate list rather than an unranked or arbitrary fallback

### Requirement: Retrieval Terms Include Preparation and State
The retrieval query for a recognized item SHALL be built from its name together with its preparation and state when those are known. Preparation and state SHALL act only as ranking hints and SHALL NOT be used to filter or exclude candidates, so that an incorrect model guess can never remove the correct food from the shortlist. An unknown preparation or state SHALL contribute no term and SHALL degrade to a name-only query rather than an empty one.

#### Scenario: Preparation improves ranking
- **WHEN** an item's preparation is known and included in the retrieval query
- **THEN** the canonical whole-food entry ranks no worse than it does for the name-only query

#### Scenario: An incorrect preparation guess is not fatal
- **WHEN** the recorded preparation does not match the food's actual preparation
- **THEN** the correct food is still present in the returned candidate shortlist, having been re-weighted rather than filtered out

#### Scenario: Unknown preparation degrades gracefully
- **WHEN** both preparation and state are unknown
- **THEN** retrieval runs on the item name alone and still returns candidates

### Requirement: Match Selection and Explicit Non-Match

The system SHALL resolve a recognized food name to a reference food by offering a retrieved candidate shortlist for selection, and SHALL record an explicit non-match rather than binding to a low-ranked candidate. Custom foods owned by the user SHALL take precedence in the shortlist over both other sources. A case-insensitive fuzzy name match against the user's custom foods — similarity at or above a fixed threshold against the item's Display Name, highest similarity wins when multiple clear the threshold — SHALL be offered as the sole candidate for that item — Open Food Facts and USDA SHALL NOT also be queried for it — though this remains a candidate offered to the same selection call as every other item, not a bypass of selection itself.

Clearing the similarity threshold SHALL NOT by itself be sufficient. Because a match here binds unconditionally and suppresses both reference databases for that item, a false positive attaches wrong macros with no alternative offered, while a false negative costs only a manual resolve — so two further conditions SHALL gate any below-perfect score. A candidate whose name differs from the Display Name only in its digits — a fat percentage, a volume, a strength — SHALL NOT match at any score below a perfect one. And a candidate SHALL NOT match on a below-perfect score unless the shorter of the two normalized names is at least ten characters long; shorter names SHALL match only when identical after normalization. The similarity score is length-normalized, so the number of differing characters it tolerates grows with the name: below ten characters any single differing letter clears the threshold, and in a short food name one letter is usually what makes it a different food rather than a misspelling. A consequence to accept deliberately: a short-named custom food — the common case in languages that name staples in one word — matches only its own name, and for a non-English Display Language, where neither reference database is queried, such an item falls through to the macro-estimate fallback exactly as any other unmatched item does. The system SHALL track, for each item's candidate shortlist, whether it is this fuzzy-name deterministic match or a fuzzy (ranked-custom-food, Open Food Facts, or USDA) shortlist, since that distinction determines whether a selected candidate binds unconditionally or only as a fallback to the item's own estimate (see `food-photo-recognition` "Macro Estimate Fallback for Unmatched Items").

When no custom food clears the fuzzy-name match threshold, the system SHALL additionally retrieve the user's own custom foods ranked by a frequency-and-recency score computed from that user's confirmed meal history (frequency weighted higher than recency, so a dish confirmed only monthly still ranks above a food used once recently; unconfirmed `pending_review` meals are excluded so an unverified automatic match cannot reinforce its own future ranking), and SHALL include the top-ranked of these in the candidate shortlist offered for selection. This addition is independent of, and additive to, the Open Food Facts/USDA routing described below: it applies whether or not the item also carries a brand, and whether the resulting shortlist otherwise comes from Open Food Facts or from USDA. Selection SHALL be instructed to prefer a candidate from the user's own custom foods when it is a reasonable match for the recognized item, since it reflects previously verified real-world macros rather than a generic reference profile. This ranked candidate remains part of the fuzzy shortlist, not the fuzzy-name deterministic one: it is a `Select` guess about which saved food matches, not an asserted identity, so a candidate selected from it is still subject to the estimate-precedence fallback described below.

When no custom food clears the fuzzy-name match threshold, and the recognizing user's Display Language is English, the system SHALL query the Open Food Facts index, using the item's name and extracted brand as the retrieval term, only when the recognized item carries a non-empty `brand`. The USDA index SHALL be queried directly, without ever querying Open Food Facts, when the recognized item carries no brand. When a brand is present, the USDA index SHALL be queried only when the Open Food Facts query returns zero candidates; when the Open Food Facts query returns one or more candidates, those SHALL be added to the shortlist offered for selection alongside any frequency/recency-ranked custom-food candidates, and the USDA index SHALL NOT be queried for that item. Candidate selection remains a text-only model call with no photo access, which is why Open Food Facts is never queried without a brand: a generic name alone gives the model no way to distinguish among differently-branded products with materially different macros, unlike the bounded variance between USDA's cooking-method variants of the same generic food. This automatic Open Food Facts query is itself a fuzzy name+brand search, not a lookup by a scanned barcode — there is no barcode-scanning capability in this pipeline — so a candidate it produces is a fuzzy shortlist candidate like any USDA one, not a deterministic identity match.

When the recognizing user's Display Language is non-English, the system SHALL NOT query Open Food Facts or USDA for that item regardless of brand — both are English-vocabulary reference databases, and no attempt is made to translate the Display Name back to English for matching. An item in this case that also found no fuzzy-name or frequency/recency-ranked custom-food candidate SHALL fall through directly to the macro-estimate fallback (see `food-photo-recognition` "Macro Estimate Fallback for Unmatched Items"), the same as an item with an empty candidate shortlist for any other reason.

The candidate shortlist offered for selection SHALL be accompanied by the recognized item's own name and (when extracted) brand, not offered as anonymous candidates keyed only by item index — the model comparing candidates needs to know what it is comparing them against, which matters most when several Open Food Facts candidates share the same brand and differ only in which specific product was actually photographed.

#### Scenario: Custom food takes precedence

- **WHEN** a user has a custom food whose Display Name fuzzy-matches a recognized item's Display Name above the similarity threshold
- **THEN** the system offers only that custom food as a candidate, does not query Open Food Facts or USDA for it, and selection resolves it

#### Scenario: A differently-worded recognition still matches via fuzzy name similarity

- **WHEN** a user has a saved custom food and a new recognition produces a Display Name that is not byte-identical to it but clears the fuzzy-name similarity threshold against it (e.g. minor phrasing differences from the vision model, or between two photos of the same dish)
- **THEN** the system offers that custom food as the sole candidate, the same as an identical-text match would have before this change

#### Scenario: Names differing only in a number are not a fuzzy-name match

- **WHEN** a recognized item's Display Name and a saved custom food's name are identical apart from the digits in them (e.g. "Milk 2%" against "Milk 3%") and their similarity clears the threshold
- **THEN** the system does not treat that custom food as the fuzzy-name match, and the item is routed as if no custom food had cleared the threshold

#### Scenario: A short name matches only itself

- **WHEN** a recognized item's Display Name and a saved custom food's name are shorter than ten characters, are not identical after normalization, and their similarity clears the threshold (e.g. "Batter" against "Butter")
- **THEN** the system does not treat that custom food as the fuzzy-name match, and — for a non-English Display Language, where neither reference database is queried — the item falls through to the macro-estimate fallback rather than binding to the wrong food

#### Scenario: A frequently-reused custom food is offered despite a differently-worded name

- **WHEN** no custom food clears the fuzzy-name match threshold against a recognized item's Display Name, and the user has a custom food that has been used in at least one confirmed meal repeatedly (or at least once a month) across their history
- **THEN** that custom food is included in the candidate shortlist offered for selection alongside any Open Food Facts or USDA candidates, even though its name does not fuzzy-match the recognized item's Display Name

#### Scenario: An unconfirmed match does not inflate its own future ranking

- **WHEN** a meal is still `pending_review` and one of its items is currently bound to a custom food via this matching process
- **THEN** that not-yet-confirmed use is excluded from the frequency/recency score computed for future candidate retrieval, until the meal is confirmed

#### Scenario: A frequently-reused custom food is preferred over a generic reference match

- **WHEN** the candidate shortlist for a recognized item contains both a frequency/recency-ranked custom food and a generic Open Food Facts or USDA candidate that could plausibly match
- **THEN** selection prefers the user's own custom food, since it reflects previously verified real-world macros

#### Scenario: A recognized brand routes matching to Open Food Facts first

- **WHEN** no custom food clears the fuzzy-name match threshold, the recognizing user's Display Language is English, and the recognized item carries a non-empty `brand`, and the Open Food Facts index returns one or more candidates for the name+brand query
- **THEN** the system offers the Open Food Facts candidates for selection, plus any frequency/recency-ranked custom-food candidates, and does not query the USDA index for that item

#### Scenario: A recognized brand with no Open Food Facts match falls back to USDA

- **WHEN** no custom food clears the fuzzy-name match threshold, the recognizing user's Display Language is English, the recognized item carries a non-empty `brand`, and the Open Food Facts query returns zero candidates
- **THEN** the system queries the USDA index and offers its candidates for selection, plus any frequency/recency-ranked custom-food candidates

#### Scenario: No recognized brand goes straight to USDA

- **WHEN** no custom food clears the fuzzy-name match threshold, the recognizing user's Display Language is English, and the recognized item's `brand` is empty
- **THEN** the system queries the USDA index directly, offers its candidates plus any frequency/recency-ranked custom-food candidates, and does not query the Open Food Facts index for that item, since there is no signal available to safely select among differently-branded Open Food Facts products

#### Scenario: A non-English Display Language skips Open Food Facts and USDA entirely

- **WHEN** no custom food clears the fuzzy-name match threshold against a recognized item, and the recognizing user's Display Language is non-English
- **THEN** the system does not query Open Food Facts or USDA for that item regardless of brand, and the item falls through to the macro-estimate fallback if it has a usable estimate, or `macro_source = none` otherwise

#### Scenario: Selection is offered the recognized item's own identity alongside its candidates

- **WHEN** the system offers a candidate shortlist for a recognized item to the selection call
- **THEN** the payload includes that item's recognized Display Name and brand (when extracted), not only its candidates, so the model can compare each candidate against what was actually recognized rather than judging the shortlist in isolation

#### Scenario: Candidate selected from a deterministic exact-name match binds unconditionally

- **WHEN** the vision model is offered the fuzzy-name custom-food candidate shortlist for a recognized item and selects it
- **THEN** the system binds the item to that custom food and scales its macros from its profile, setting `macro_source = reference`, regardless of whether the item also carries an estimated nutrient profile

#### Scenario: Candidate selected from shortlist

- **WHEN** the vision model is given a fuzzy shortlist (ranked custom food, Open Food Facts, or USDA candidates) for a recognized item and selects one
- **THEN** the system uses that candidate's macros with `macro_source = reference` only if the item has no usable estimated profile (see `food-photo-recognition` "Macro Estimate Fallback for Unmatched Items"); when a usable estimate is present, the system uses the estimate instead and does not bind to the selected candidate

#### Scenario: No suitable candidate

- **WHEN** no candidate in the shortlist is a suitable match for the recognized item
- **THEN** the system falls back to the macro estimate produced by Recognize for that item (see `food-photo-recognition` "Macro Estimate Fallback for Unmatched Items"), storing `macro_source = estimated` and surfacing it in the review UI, rather than binding it to the highest-ranked candidate

### Requirement: Operator-Run USDA Import
The system SHALL provide an `hcw import-usda` command that downloads the USDA Foundation and SR Legacy datasets and builds the local SQLite FTS5 database. The import SHALL write to a temporary file, validate a minimum expected row count, and only then atomically replace the existing database, so a failed or partial import leaves the previous database in service. The system SHALL NOT run any scheduled or background dataset updater.

#### Scenario: Successful import
- **WHEN** an operator runs the import command and the download and index build succeed
- **THEN** the new database atomically replaces the previous one and the command reports the imported row count

#### Scenario: Failed import leaves previous data serving
- **WHEN** the download fails, or the built database contains fewer rows than the minimum expected count
- **THEN** the command exits non-zero, the temporary file is discarded, and the previously imported database remains in place and queryable

#### Scenario: Search before any import
- **WHEN** a food search runs before any USDA import has been performed
- **THEN** the system returns an empty USDA candidate list together with a flag indicating the reference database is absent, and the enclosing meal analysis still completes with its items recorded as unmatched

### Requirement: Custom User Food Entry and Correction
The system SHALL allow users to store custom foods with per-100g macro profiles, for example from packaged food labels, scoped to the owning user. Custom food names SHALL be unique per user, and the system SHALL provide update and delete alongside create and list.

Both properties follow from precedence: a custom food shadows every USDA entry sharing its name. Without per-user name uniqueness, two custom foods called "yogurt" make the winning match arbitrary. Without update and delete, a custom food saved with a mistyped macro value permanently poisons matching for that name with no in-app way to correct it.

Updating a custom food to a different name SHALL clear its Canonical Name, for the same reason a hand-corrected and renamed item loses one (see `food-nutrition-logging` "Item Resolution"): the recorded English gloss described the old name, and pairing it with the new one in Expert Mode would assert an identity the system never established. Names SHALL be compared case-insensitively, so a pure capitalization fix does not cost the user their Canonical Name. Unlike a meal item's PATCH, this endpoint has no rename-only mode — it is a full-resource update — so every name change here is the identity-replacing case.

#### Scenario: Custom food creation
- **WHEN** a user saves a custom food with a name and per-100g macro values
- **THEN** the system stores it scoped to that user and makes it available to subsequent searches and matching

#### Scenario: Duplicate custom food name rejected
- **WHEN** a user saves a custom food whose name matches one they already own, ignoring case
- **THEN** the system returns HTTP 409 and does not create a second entry

#### Scenario: Correct a mistyped custom food
- **WHEN** the owner updates a custom food's macro values
- **THEN** subsequent matches use the corrected values

#### Scenario: Renaming a custom food clears its Canonical Name
- **WHEN** the owner updates a custom food that carries a Canonical Name, supplying a name that differs from its current one by more than capitalization
- **THEN** the system stores the new name and clears the Canonical Name, rather than showing the new name paired with the previous name's English gloss in Expert Mode

#### Scenario: Recapitalizing a custom food keeps its Canonical Name
- **WHEN** the owner updates a custom food that carries a Canonical Name, supplying a name that differs from its current one only in capitalization
- **THEN** the system stores the new name and leaves the Canonical Name in place, since a capitalization fix is not an identity change

#### Scenario: Delete a custom food restores USDA matching
- **WHEN** the owner deletes a custom food
- **THEN** it no longer shadows USDA entries and a later search for that name returns USDA candidates

#### Scenario: Cross-user custom food isolation
- **WHEN** a user searches for foods
- **THEN** the results include only their own custom foods and never another user's

#### Scenario: Cross-user custom food mutation
- **WHEN** a user attempts to update or delete a custom food owned by another user
- **THEN** the system returns HTTP 404 and the record is unchanged

