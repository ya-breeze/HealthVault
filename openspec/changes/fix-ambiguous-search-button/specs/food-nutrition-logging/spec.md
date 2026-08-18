## ADDED Requirements

### Requirement: Item Resolution Mode Controls Are Distinctly Labeled

The item-resolution UI SHALL label its search-mode selector and its search-submit control with visibly different text, so a user cannot mistake the mode selector (which only switches which panel is shown and performs no search) for the control that actually triggers a food search. This applies regardless of which mode is selected by default.

#### Scenario: Search mode is already selected when the panel opens

- **WHEN** the item-resolution panel opens with search mode already active
- **THEN** the mode selector and the search-submit control are not labeled with the identical text, so clicking the mode selector cannot be mistaken for triggering a search
