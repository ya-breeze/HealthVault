## MODIFIED Requirements

### Requirement: Match Selection and Explicit Non-Match

The system SHALL resolve a recognized food name to a reference food by offering a retrieved candidate shortlist for selection, and SHALL record an explicit non-match rather than binding to a low-ranked candidate. Custom foods owned by the user SHALL take precedence over both other sources: a case-insensitive exact name match against the user's custom foods SHALL be selected without consulting either the Open Food Facts or USDA index.

When a custom food does not match, the system SHALL query the Open Food Facts index, using the item's name and extracted brand as the retrieval term, only when the recognized item carries a non-empty `brand`. The USDA index SHALL be queried directly, without ever querying Open Food Facts, when the recognized item carries no brand. When a brand is present, the USDA index SHALL be queried only when the Open Food Facts query returns zero candidates; when the Open Food Facts query returns one or more candidates, those SHALL be the shortlist offered for selection and the USDA index SHALL NOT be queried for that item. Candidate selection remains a text-only model call with no photo access, which is why Open Food Facts is never queried without a brand: a generic name alone gives the model no way to distinguish among differently-branded products with materially different macros, unlike the bounded variance between USDA's cooking-method variants of the same generic food.

The candidate shortlist offered for selection SHALL be accompanied by the recognized item's own name and (when extracted) brand, not offered as anonymous candidates keyed only by item index — the model comparing candidates needs to know what it is comparing them against, which matters most when several Open Food Facts candidates share the same brand and differ only in which specific product was actually photographed.

#### Scenario: Custom food takes precedence

- **WHEN** a user has a custom food whose name exactly matches a recognized item name, ignoring case
- **THEN** the system selects that custom food and does not substitute a USDA or Open Food Facts entry, and the selection is unambiguous because custom food names are unique per user

#### Scenario: A recognized brand routes matching to Open Food Facts first

- **WHEN** no custom food matches and the recognized item carries a non-empty `brand`, and the Open Food Facts index returns one or more candidates for the name+brand query
- **THEN** the system offers only the Open Food Facts candidates for selection and does not query the USDA index for that item

#### Scenario: A recognized brand with no Open Food Facts match falls back to USDA

- **WHEN** no custom food matches, the recognized item carries a non-empty `brand`, and the Open Food Facts query returns zero candidates
- **THEN** the system queries the USDA index and offers its candidates for selection

#### Scenario: No recognized brand goes straight to USDA

- **WHEN** no custom food matches and the recognized item's `brand` is empty
- **THEN** the system queries the USDA index directly and does not query the Open Food Facts index for that item, since there is no signal available to safely select among differently-branded Open Food Facts products

#### Scenario: Selection is offered the recognized item's own identity alongside its candidates

- **WHEN** the system offers a candidate shortlist for a recognized item to the selection call
- **THEN** the payload includes that item's recognized name and brand (when extracted), not only its candidates, so the model can compare each candidate against what was actually recognized rather than judging the shortlist in isolation

#### Scenario: Candidate selected from shortlist

- **WHEN** the vision model is given a candidate shortlist for a recognized item and selects one
- **THEN** the system binds the item to the selected food (via `fdc_id` or `off_code`, matching whichever index the candidate came from) and scales its macros from that profile

#### Scenario: No suitable candidate

- **WHEN** no candidate in the shortlist is a suitable match for the recognized item
- **THEN** the system stores the item with `macro_source = none` and surfaces it in the review UI, which resolves it via `PATCH /api/food/meals/{id}/items/{item_id}`, rather than binding it to the highest-ranked candidate
