## MODIFIED Requirements

### Requirement: Match Selection and Explicit Non-Match

The system SHALL resolve a recognized food name to a reference food by offering a retrieved candidate shortlist for selection, and SHALL record an explicit non-match rather than binding to a low-ranked candidate. Custom foods owned by the user SHALL take precedence in the shortlist over both other sources. A case-insensitive exact name match against the user's custom foods SHALL be offered as the sole candidate for that item — Open Food Facts and USDA SHALL NOT also be queried for it — though this remains a candidate offered to the same selection call as every other item, not a bypass of selection itself; because custom food names are unique per user, this candidate is unambiguous in practice even though selection is not structurally prevented from rejecting it.

When a custom food does not match by exact name, the system SHALL additionally retrieve the user's own custom foods ranked by a frequency-and-recency score computed from that user's confirmed meal history (frequency weighted higher than recency, so a dish confirmed only monthly still ranks above a food used once recently; unconfirmed `pending_review` meals are excluded so an unverified automatic match cannot reinforce its own future ranking), and SHALL include the top-ranked of these in the candidate shortlist offered for selection. This addition is independent of, and additive to, the Open Food Facts/USDA routing described below: it applies whether or not the item also carries a brand, and whether the resulting shortlist otherwise comes from Open Food Facts or from USDA. Selection SHALL be instructed to prefer a candidate from the user's own custom foods when it is a reasonable match for the recognized item, since it reflects previously verified real-world macros rather than a generic reference profile.

When a custom food does not exactly match, the system SHALL query the Open Food Facts index, using the item's name and extracted brand as the retrieval term, only when the recognized item carries a non-empty `brand`. The USDA index SHALL be queried directly, without ever querying Open Food Facts, when the recognized item carries no brand. When a brand is present, the USDA index SHALL be queried only when the Open Food Facts query returns zero candidates; when the Open Food Facts query returns one or more candidates, those SHALL be added to the shortlist offered for selection alongside any frequency/recency-ranked custom-food candidates, and the USDA index SHALL NOT be queried for that item. Candidate selection remains a text-only model call with no photo access, which is why Open Food Facts is never queried without a brand: a generic name alone gives the model no way to distinguish among differently-branded products with materially different macros, unlike the bounded variance between USDA's cooking-method variants of the same generic food.

The candidate shortlist offered for selection SHALL be accompanied by the recognized item's own name and (when extracted) brand, not offered as anonymous candidates keyed only by item index — the model comparing candidates needs to know what it is comparing them against, which matters most when several Open Food Facts candidates share the same brand and differ only in which specific product was actually photographed.

#### Scenario: Custom food takes precedence

- **WHEN** a user has a custom food whose name exactly matches a recognized item name, ignoring case
- **THEN** the system offers only that custom food as a candidate, does not query Open Food Facts or USDA for it, and selection resolves it unambiguously since custom food names are unique per user

#### Scenario: A frequently-reused custom food is offered despite a differently-worded name

- **WHEN** no custom food exactly matches a recognized item's name, and the user has a custom food that has been used in at least one confirmed meal repeatedly (or at least once a month) across their history
- **THEN** that custom food is included in the candidate shortlist offered for selection alongside any Open Food Facts or USDA candidates, even though its name does not exactly match the recognized item's name

#### Scenario: An unconfirmed match does not inflate its own future ranking

- **WHEN** a meal is still `pending_review` and one of its items is currently bound to a custom food via this matching process
- **THEN** that not-yet-confirmed use is excluded from the frequency/recency score computed for future candidate retrieval, until the meal is confirmed

#### Scenario: A frequently-reused custom food is preferred over a generic reference match

- **WHEN** the candidate shortlist for a recognized item contains both a frequency/recency-ranked custom food and a generic Open Food Facts or USDA candidate that could plausibly match
- **THEN** selection prefers the user's own custom food, since it reflects previously verified real-world macros

#### Scenario: A recognized brand routes matching to Open Food Facts first

- **WHEN** no custom food matches by exact name and the recognized item carries a non-empty `brand`, and the Open Food Facts index returns one or more candidates for the name+brand query
- **THEN** the system offers the Open Food Facts candidates for selection, plus any frequency/recency-ranked custom-food candidates, and does not query the USDA index for that item

#### Scenario: A recognized brand with no Open Food Facts match falls back to USDA

- **WHEN** no custom food matches by exact name, the recognized item carries a non-empty `brand`, and the Open Food Facts query returns zero candidates
- **THEN** the system queries the USDA index and offers its candidates for selection, plus any frequency/recency-ranked custom-food candidates

#### Scenario: No recognized brand goes straight to USDA

- **WHEN** no custom food matches by exact name and the recognized item's `brand` is empty
- **THEN** the system queries the USDA index directly, offers its candidates plus any frequency/recency-ranked custom-food candidates, and does not query the Open Food Facts index for that item, since there is no signal available to safely select among differently-branded Open Food Facts products

#### Scenario: Selection is offered the recognized item's own identity alongside its candidates

- **WHEN** the system offers a candidate shortlist for a recognized item to the selection call
- **THEN** the payload includes that item's recognized name and brand (when extracted), not only its candidates, so the model can compare each candidate against what was actually recognized rather than judging the shortlist in isolation

#### Scenario: Candidate selected from shortlist

- **WHEN** the vision model is given a candidate shortlist for a recognized item and selects one
- **THEN** the system binds the item to the selected food (via `fdc_id`, `off_code`, or the custom food's id, matching whichever source the candidate came from) and scales its macros from that profile

#### Scenario: No suitable candidate

- **WHEN** no candidate in the shortlist is a suitable match for the recognized item
- **THEN** the system falls back to the macro estimate produced by Recognize for that item (see `food-photo-recognition` "Macro Estimate Fallback for Unmatched Items"), storing `macro_source = estimated` and surfacing it in the review UI, rather than binding it to the highest-ranked candidate
