## ADDED Requirements

### Requirement: Toast Notification System
The frontend SHALL provide a toast/notification system consisting of a context provider mounted once at the application root and a hook usable by any client component to show a transient message with a `success` or `error` variant. A shown toast SHALL auto-dismiss after approximately 3 seconds and SHALL also be dismissible by explicit user action before that. Multiple toasts triggered close together SHALL stack rather than replace one another.

#### Scenario: Toast auto-dismisses
- **WHEN** a component shows a toast
- **THEN** the toast is visible immediately and disappears on its own after approximately 3 seconds without further user action

#### Scenario: Toast is manually dismissible
- **WHEN** a shown toast is still visible
- **THEN** the user can dismiss it before the auto-dismiss timer elapses

#### Scenario: Multiple toasts stack
- **WHEN** a second toast is triggered while an earlier one is still visible
- **THEN** both are visible at once, not just the most recent one

### Requirement: Food Review Mutation Feedback
The food review page (reached from both the upload flow and the meal history page, since both route to the same review page for a given meal) SHALL show a toast for every mutation issued through its shared update path: adding an item, editing an item's weight or resolved food, deleting an item, editing the meal's name or logged time, reanalyzing with a hint, retrying a failed analysis, submitting clarification answers, and confirming a meal. A successful mutation SHALL show a `success` toast identifying what happened (e.g. "Item added", "Item removed", "Reanalysis complete", "Meal updated", "Meal confirmed"); a mutation whose action isn't otherwise labeled SHALL fall back to a generic "Saved" success toast. A failed mutation SHALL show an `error` toast. Existing inline error text rendered by individual fields/controls SHALL remain unchanged — the toast is in addition to, not a replacement for, that inline feedback.

#### Scenario: Adding an item shows a success toast
- **WHEN** the user adds an item to a meal and the request succeeds
- **THEN** a success toast is shown

#### Scenario: Editing an item's weight shows a success toast
- **WHEN** the user changes an item's weight and the update succeeds
- **THEN** a success toast is shown

#### Scenario: Deleting an item shows a success toast
- **WHEN** the user deletes an item and the request succeeds
- **THEN** a success toast is shown

#### Scenario: Editing meal name or logged time shows a success toast
- **WHEN** the user edits the meal's name or logged time and the update succeeds
- **THEN** a success toast is shown

#### Scenario: Reanalyzing with a hint shows a success toast
- **WHEN** the user submits a hint and reanalysis succeeds
- **THEN** a success toast is shown

#### Scenario: Confirming a meal shows a success toast
- **WHEN** the user confirms a meal and the request succeeds
- **THEN** a success toast is shown

#### Scenario: A failed mutation shows an error toast without removing inline error text
- **WHEN** any of the above mutations fails
- **THEN** an error toast is shown, and any inline error text the failing component already renders is still shown alongside it
