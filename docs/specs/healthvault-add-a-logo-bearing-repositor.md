# Add a logo-bearing repository landing README
Idea: ya-breeze/idea-forge#589

## Why

HealthVault has no root `README.md`, so its repository landing page provides neither a project identity nor an introduction for readers arriving at the codebase. The only existing README is `frontend/README.md`, which is generic Create Next App scaffold text scoped to the frontend and does not describe HealthVault.

`CONTEXT.md` identifies HealthVault as a personal health-tracking application whose food-logging capability can recognize a photographed meal with AI and track its nutrition over time. The owner-selected logo is already present at `frontend/app/icon.png`, alongside the deployed App Router and manifest icon bundle. This repository-level documentation should land now that the approved visual identity exists, without reopening the completed icon work split from issue #529.

## How

Create a concise root `README.md` whose first visible element displays `frontend/app/icon.png` with meaningful alt text and a restrained explicit width suitable for GitHub rendering, followed by the `HealthVault` heading. Reference the existing file through a repository-relative path rather than copying or generating another image, so the landing page and application continue to share the owner-supplied artwork.

Describe HealthVault in language consistent with `CONTEXT.md`, including its personal health tracking and AI-assisted photographed-meal nutrition logging. Add a compact repository map for the Go backend in `backend/`, the Next.js application in `frontend/`, and the Playwright suite in `e2e/`. Link readers to `CONTEXT.md` for the project vocabulary and behavior and to `docs/adr/` for architectural decisions. Keep the README focused on repository orientation; do not turn it into a deployment runbook or repeat the stale scaffold instructions from `frontend/README.md`.

Deliberately exclude edits to `frontend/app/icon.png`, `frontend/app/apple-icon.png`, `frontend/app/favicon.ico`, `frontend/public/icon-192.png`, `frontend/public/icon-512.png`, `frontend/public/icon-512-maskable.png`, and `frontend/app/manifest.ts`. Also exclude application UI branding, generated artwork, changes to the frontend scaffold README, hosted-environment details, credentials, public hostnames, Cloudflare Access policy, and any `dogfood` or `prod` deployment work.

## Validation Commands
- `make lint`
- `make test`
- `make test-e2e`

## Ground rules
This spec is implemented by an automated pass running unattended. **There is no approval step and nothing is waiting for one** — do not look for a tick, a marker, or a sign-off anywhere, and do not wait for one.

Tick the boxes in this file as the work is completed; they are the record of progress, and the pipeline reads them to decide whether the change is finished.

Out of scope, deliberately: do NOT mark the pull request ready for review and do NOT call a forge merge API. Implementation marks the pull request ready only after the task list is complete. Afterward Completion may ask the Store to perform Automatic Merge only when the planner and final implementation agent authorized the exact result. Leave the pull request in a state worth reading.

### Task 1: Add the repository landing README

- [ ] Create root `README.md` with the existing `frontend/app/icon.png` displayed at the top using a repository-relative reference, meaningful alt text, and a restrained display width, followed by a `HealthVault` heading.
- [ ] Add a concise description consistent with `CONTEXT.md`, identifying HealthVault as a personal health-tracking application and summarizing its AI-assisted photographed-meal nutrition tracking.
- [ ] Add a compact repository map linking to `backend/`, `frontend/`, and `e2e/`, and documentation links to `CONTEXT.md` and `docs/adr/`.
- [ ] Confirm every relative image and documentation link resolves from the root README and that the final change leaves the existing logo, favicon, Apple icon, manifest, and public icon bundle byte-for-byte unchanged.
- [ ] Run the validation commands and address any failures caused by the README change.
- [ ] Mark completed
