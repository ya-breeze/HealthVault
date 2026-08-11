## MODIFIED Requirements

### Requirement: Match Selection and Explicit Non-Match

The system SHALL resolve a recognized food name to a reference food by offering a retrieved candidate shortlist for selection, and SHALL record an explicit non-match rather than binding to a low-ranked candidate. Custom foods owned by the user SHALL take precedence over both other sources: a case-insensitive exact name match against the user's custom foods SHALL be selected without consulting either the Open Food Facts or USDA index.

When a custom food does not match, the system SHALL query the Open Food Facts index before the USDA index. The USDA index SHALL be queried only when the Open Food Facts query returns zero candidates. When the Open Food Facts query returns one or more candidates, those SHALL be the shortlist offered for selection and the USDA index SHALL NOT be queried for that item.

#### Scenario: Custom food takes precedence

- **WHEN** a user has a custom food whose name exactly matches a recognized item name, ignoring case
- **THEN** the system selects that custom food and does not substitute a USDA or Open Food Facts entry, and the selection is unambiguous because custom food names are unique per user

#### Scenario: Open Food Facts candidates take precedence over USDA

- **WHEN** no custom food matches and the Open Food Facts index returns one or more candidates for a recognized item
- **THEN** the system offers only the Open Food Facts candidates for selection and does not query the USDA index for that item

#### Scenario: USDA is queried only as a fallback

- **WHEN** no custom food matches and the Open Food Facts index returns zero candidates for a recognized item
- **THEN** the system queries the USDA index and offers its candidates for selection

#### Scenario: Candidate selected from shortlist

- **WHEN** the vision model is given a candidate shortlist for a recognized item and selects one
- **THEN** the system binds the item to the selected food (via `fdc_id` or `off_code`, matching whichever index the candidate came from) and scales its macros from that profile

#### Scenario: No suitable candidate

- **WHEN** no candidate in the shortlist is a suitable match for the recognized item
- **THEN** the system stores the item with `macro_source = none` and surfaces it in the review UI, which resolves it via `PATCH /api/food/meals/{id}/items/{item_id}`, rather than binding it to the highest-ranked candidate
