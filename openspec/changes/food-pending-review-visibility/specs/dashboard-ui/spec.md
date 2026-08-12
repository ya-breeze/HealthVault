## ADDED Requirements

### Requirement: Needs-attention indicator
The dashboard SHALL show an indicator when the authenticated user has one or more logged meals needing attention (as counted by `GET /api/food/meals/needs-attention-count`), stating the count and linking to the meal history page (`/food/history/`). The indicator SHALL NOT be rendered when the count is zero.

#### Scenario: Indicator shown when meals need attention
- **GIVEN** the resolved user has 2 meals needing attention
- **WHEN** they load the dashboard
- **THEN** an indicator is visible stating that 2 meals need attention

#### Scenario: Indicator hidden when nothing needs attention
- **GIVEN** the resolved user has no meals needing attention (none logged, or all confirmed)
- **WHEN** they load the dashboard
- **THEN** no needs-attention indicator is rendered

#### Scenario: Indicator links to meal history
- **WHEN** a user clicks the needs-attention indicator
- **THEN** the system navigates to `/food/history/`
