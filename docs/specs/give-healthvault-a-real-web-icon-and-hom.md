# Explore and select HealthVault's web icon
Idea: ya-breeze/idea-forge#383

## Why

HealthVault currently exposes only `frontend/app/favicon.ico`, the generic icon left by the original Next.js scaffold. The repository still contains the scaffold's unused-looking `frontend/public/file.svg`, `globe.svg`, `next.svg`, `vercel.svg`, and `window.svg`, and `frontend/README.md` still identifies the frontend as a create-next-app project. Meanwhile, the product itself has developed a recognizable Instrument Panel identity: `frontend/app/globals.css` defines teal accents (`#0f9c8c` in light mode and `#4fd8c4` in dark mode), restrained neutral surfaces (`#f7f8fa` and `#10141a`), and a purpose-built line-icon system documented in `frontend/components/icons.tsx`.

The browser and home-screen identity has not caught up with that interface. A starter favicon makes HealthVault difficult to distinguish in tabs, bookmarks, and installed shortcuts, and there is no selected source mark from which the favicon, Apple touch icon, and 192/512 application assets can be derived consistently. Choosing that source mark is a visual-design decision that should be reviewable independently of the mechanical Next.js metadata integration.

## How

This first split lands the design groundwork: several original vector candidates, a rendered comparison at the actual target sizes, and an explicit default selection. Runtime files such as `frontend/app/layout.tsx`, `frontend/app/favicon.ico`, and `frontend/public/` remain unchanged in this part; the follow-up can then derive every production asset from one reviewed source rather than improvising variants while wiring metadata. The automated implementation does not wait for approval: it evaluates the candidates against the documented rubric, selects the strongest default, and records the decision and trade-offs.

Create `docs/investigations/idea-383-healthvault-icon.md` as the decision record and retain the editable candidate SVGs under a new `docs/assets/healthvault-icon/` directory. Develop at least three materially different, semantically named concepts combining secure storage with personal health data—for example a vault dial carrying a pulse gesture, a compact HV/vault-door monogram, and a protected health-record chamber—rather than three color variations of one drawing. Favor bold geometric silhouettes, generous negative space, and very few internal features so the mark survives at 16px. Avoid standalone hearts, medical crosses, shields, padlocks, text, gradients, hairline detail, copied marks, and the generic house drawn by `HomeIcon`; those either collapse at favicon scale or communicate a stock category instead of this product.

Use the established palette rather than introducing a new brand system. The comparison must include the light and dark application surfaces from `frontend/app/globals.css`, both accent values, and a stable high-contrast container treatment where necessary so the same artwork remains identifiable against light and dark browser chrome. Keep critical geometry inside a conservative central safe zone for circular and rounded-square home-screen masks.

Provide a self-contained `docs/assets/healthvault-icon/comparison.html` and a captured `comparison.png`. Show every candidate at 16px and 32px in favicon-like tab treatments and at 180px, 192px, and 512px in square, rounded-square, and circular home-screen masks on both light and dark backgrounds. Evaluate the rendered results at their displayed sizes rather than judging only enlarged vector paths. The investigation must score recognizability, health/vault relevance, silhouette distinctiveness, light/dark contrast, small-size legibility, mask safety, and ease of deriving monochrome and raster variants. Name one SVG unambiguously as the selected default, explain why it wins, record why each alternative was rejected, and document the intended production colors and safe-zone rules for the follow-up.

This part deliberately excludes production icons, manifest metadata, changes to the existing `viewport` export, removal of starter assets, header or login-page logo treatments, service workers, offline behavior, install prompts, and broader palette or typography changes. None of those is needed to review the visual decision, and the metadata/assets work has its own coherent browser-validation surface. No hostname, Cloudflare Access policy, production stack, credential, or other owner-only infrastructure change is required.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Establish the icon brief and evaluation rubric

- [x] Create `docs/investigations/idea-383-healthvault-icon.md` and cite the current favicon-only state in `frontend/app/`, the starter files in `frontend/public/`, the metadata and safe-area viewport exports in `frontend/app/layout.tsx`, and the Instrument Panel tokens in `frontend/app/globals.css`.
- [x] Record the light background `#f7f8fa`, dark background `#10141a`, light accent `#0f9c8c`, and dark accent `#4fd8c4` as the palette from which candidates must be built.
- [x] Define the comparison rubric covering recognizability, combined health/vault meaning, distinctive silhouette, 16px and 32px legibility, light/dark contrast, circular and rounded-square mask safety, and suitability for SVG, ICO, Apple touch, and 192/512 derivatives.
- [x] Document the exclusions on stock heart, cross, shield, padlock, house, text, gradient, and fine-detail treatments so the exploration remains specific to HealthVault.
- [x] Mark completed

### Task 2: Produce and retain distinct vector candidates

- [x] Add at least three semantically named candidate SVG files under `docs/assets/healthvault-icon/`, with genuinely different geometry rather than palette-only variants.
- [x] Make every candidate a standalone square SVG with a consistent `viewBox`, no embedded font, no external resource, no copied artwork, and critical geometry kept inside the documented mask-safe area.
- [x] Use only the documented HealthVault palette plus transparency or a necessary neutral container, and keep each silhouette readable without relying on color alone.
- [x] Inspect each source at 16px before accepting it into the comparison and simplify any paths or gaps that merge, disappear, or become ambiguous at that size.
- [x] Mark completed

### Task 3: Compare candidates and select the default

- [ ] Add `docs/assets/healthvault-icon/comparison.html` as a self-contained comparison grid showing every candidate at 16px, 32px, 180px, 192px, and 512px on the documented light and dark backgrounds.
- [ ] Include favicon-like tab contexts plus square, rounded-square, and circular home-screen masks, without scaling labels or surrounding decoration in ways that obscure the actual rendered sizes.
- [ ] Capture the completed grid as `docs/assets/healthvault-icon/comparison.png` so the light/dark and mask comparison is directly reviewable with the committed change.
- [ ] Score the candidates against the rubric in `docs/investigations/idea-383-healthvault-icon.md`, select one named SVG as the default, and record concrete strengths and failure modes for every candidate.
- [ ] Document the selected mark's production palette, clear-space and safe-zone rules, minimum-detail behavior at favicon sizes, and which features must remain invariant when raster derivatives are produced.
- [ ] Mark completed

### Task 4: Review and validate the design package

- [ ] Review the candidate sources and comparison together at their intended display sizes, correcting clipping, uneven optical weight, insufficient contrast, or mask-unsafe geometry before finalizing the decision record.
- [ ] Confirm this split has not changed `frontend/app/layout.tsx`, replaced `frontend/app/favicon.ico`, added production assets under `frontend/public/`, or removed any starter asset.
- [ ] Confirm all candidate artwork remains retained and the decision record points to filenames that exist, with one and only one selected default.
- [ ] Run `make lint`, `make test`, and `make test-e2e`, and resolve any change-caused failure without targeting a dogfood or production stack.
- [ ] Recheck the final diff for temporary exports, duplicate captures, external-resource references, and unrelated files.
- [ ] Mark completed
