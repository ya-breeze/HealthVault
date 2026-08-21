# ADR-001: Dashboard Card Visibility Stored Inline With Order

## Status
Proposed

## Context and Problem Statement

The dashboard's vitals grid already lets a user reorder its 8 Vital Cards, persisted as `UserSettings.dashboard_order` (`[]string` of metric types). We're adding the ability to hide/show individual Cards too. When a hidden Card is re-shown, where should it reappear — back where it was, or wherever is convenient to implement?

## Decision Drivers

- A user who hides a Card and later re-shows it should not be surprised to find it moved
- New Card types (introduced by later phases, e.g. Food Cards) must default to visible without a migration step
- Avoid a second source of truth that can drift out of sync with the order list

## Considered Options

- **Separate `hidden_cards` list alongside `dashboard_order`** — simplest one-line diff, but a Card removed from `dashboard_order` when hidden loses its position; re-showing it appends it at the end instead of restoring its place. Two lists can also drift (e.g. a type present in both, or in neither).
- **Single ordered list of `{type, hidden}`** — one structure is both the order and the visibility state; hiding never removes a Card from the list, just flips a flag.

## Decision Outcome

Chosen: **replace `UserSettings.dashboard_order` (`[]string`) with a single ordered list of `{type: string, hidden: bool}`.** Position and visibility live in the same structure, so hiding and re-showing a Card is a pure flag flip with no positional side effect. A Card type absent from the list (including any newly introduced type) is treated as visible and appended at its default position — this is what makes new Card types default-visible with no explicit migration.

### Consequences

- Existing stored `dashboard_order` values need a one-time read-path migration to the new shape (`hidden: false` for every entry already present).
- This phase applies only to the 8 Vital Cards in the grid. The dashboard's other three sections (needs-attention banner, Log Food row, More Data row) are hardcoded JSX blocks, not Cards, and are explicitly out of scope — folding them into this registry is a separate future decision, not assumed here.
