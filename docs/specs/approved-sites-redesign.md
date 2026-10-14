# Implement the approved HealthVault Sites redesign

Idea: https://ideaforge.ikoro.in/idea/1069

## Why

The owner approved the responsive Sites design and asked to implement it. The current dashboard and food pages use inconsistent layouts. Web nutrition omits fiber already returned by the backend and shown by Android.

## How

Use the approved private prototype at https://healthvault-design-20261014.ya-breeze.chatgpt.site (source 8ab6da467b6c4f9f68c613eca3f42faa44b4c092) as the visual and interaction reference. Build shared responsive chrome, spacious cards, clear headings and consistent forms. Keep real product behavior, English/Russian localization, dark mode, all existing destinations, attention states, loading/errors and data-integrity semantics.

Keep the existing settings write queue, card order/visibility structure and registry IDs, especially logging_gap. Use a compact gear in the dashboard title row and existing inline eyes/arrows. Hide redundant dashboard food shortcuts on phones while keeping both photo and manual entry reachable from bottom navigation. Remove promotional habits content. Add fiber to today's nutrition, food-day totals and meal summaries. Respect nullable/unknown fiber; keep independent fiber target when calorie goals are unavailable. Preserve 7-day advice/basis and 28-day logging agreement inside the one nutrition card.

Adapt the prototype to actual product capabilities; do not ship synthetic measurements, simulated actions, invented targets or prototype-only controls. Preserve all eight existing present Vital Cards and the independent More Data preference. No backend schema, health algorithm, Android, merge or production deployment changes.

## Validation Commands

- make test-frontend
- make test-backend
- make lint
- cd frontend && npx tsc --noEmit && npm run build
- BASE_URL=<confirmed hcw-wip URL> make test-e2e E2E_ARGS=--retries=0

### Task 1: Shared responsive visual system and navigation
- [ ] Implement sidebar/header/mobile chrome and shared page surfaces matching the approved design.
- [ ] Preserve every existing destination/control, bilingual text, dark mode and safe-area clearance.
- [ ] Mark completed

### Task 2: Dashboard and fiber parity
- [ ] Implement compact gear, inline visibility/order, responsive dashboard cards and no duplicate mobile shortcuts.
- [ ] Show dietary fiber with its independent target alongside today's macros, including unavailable targets and unknown consumption.
- [ ] Preserve attention, presence, loading/error, healthiness/advice/basis/chat and logging-gap semantics.
- [ ] Mark completed

### Task 3: Food workflows and remaining pages
- [ ] Restyle food history, upload/manual entry, review and Custom Foods, preserving real actions and corrections.
- [ ] Add fiber to day/meal summaries without counting pending meals or unknown values as zero.
- [ ] Restyle settings, import, metric details, weather and login consistently while preserving behavior.
- [ ] Mark completed

### Task 4: Validation, review and handoff
- [ ] Add focused behavior regressions for fiber and responsive dashboard controls.
- [ ] Pass unit/static/build gates and native plus best-effort Claude peer review.
- [ ] Deploy reviewed branch to confirmed WIP and validate real deployed E2E plus desktop/mobile design parity.
- [ ] Verify fixtures/preferences restored; record exact revision, checks and draft PR linked to the Idea.
- [ ] Mark completed
