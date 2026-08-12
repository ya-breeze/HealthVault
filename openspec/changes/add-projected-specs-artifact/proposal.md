## Why

OpenSpec delta specs under `openspec/changes/<id>/specs/<capability>/spec.md` use `## ADDED Requirements` / `## MODIFIED Requirements` / `## REMOVED Requirements` sections. A `MODIFIED` requirement records only its new text, with no reference to the requirement it replaces, and the delta file itself is new in git — so a PR diff shows no old-to-new comparison. Reviewers cannot see what a spec will actually read after the change lands; they only see isolated new prose. Deltas are merged into `openspec/specs/` at `openspec archive`, which in this repo's workflow happens right before merge, i.e. too late to review the merged wording as part of the PR diff.

## What Changes

- Add a generator script (`scripts/generate-projected-specs.sh`) that materializes what each *touched capability's* `openspec/specs/<capability>/spec.md` will read after the current branch's touched changes are archived, writing both the full post-archive text and an explicit unified diff against the current spec to `openspec/specs.projected/<capability>/`. Scoped to touched capabilities only — not a full copy of `openspec/specs/` — so the PR diff stays focused instead of showing every capability in the repo as newly added.
- Add a GitHub Actions workflow that regenerates the projection on every PR touching `openspec/**` and fails if the committed copy is stale or entirely missing (comparing against the staged index, not a plain working-tree diff, so an omitted/untracked projection is caught too). This is the first GitHub Actions workflow in this repository.
- Add a `projected-specs` Makefile target wiring the script into the existing task runner.
- Add `.gitattributes` entries marking `openspec/specs.projected/**` as generated, so it collapses in PR review and is excluded from language stats.
- Document the new artifact: what it is, that it's never hand-edited, how to regenerate it, and that `openspec/specs.projected/` is reviewed for final wording while the delta under `openspec/changes/<id>/specs/` is reviewed for intent.
- Establish and document a lifecycle invariant: `openspec/specs.projected/` exists only while a branch has open (unarchived) deltas it touched. Once a change is archived — which this repo's workflow always does in the same PR, before merge — regenerating produces an empty projection, and that removal is committed as part of the archive step, so `main` never carries projected-spec content.

## Capabilities

### New Capabilities
- `projected-specs-generation`: generating, validating in CI, and cleaning up a materialized `openspec/specs.projected/` view of a branch's in-flight OpenSpec deltas, so reviewers can see final spec wording in the PR diff without changing when deltas actually merge into `openspec/specs/`.

### Modified Capabilities
(none — this does not change behavior of any existing capability's requirements)

## Impact

- New files: `scripts/generate-projected-specs.sh`, `.github/workflows/openspec-projected-specs.yml`, a short doc under `openspec/specs.projected/README.md`, `.gitattributes` additions, a new `projected-specs` Makefile target.
- No changes to `openspec/specs/` or `openspec/changes/` other than this change's own artifacts.
- No changes to application code (Go backend, Next.js frontend) or its runtime behavior.
- Adds a new CI dependency: `@fission-ai/openspec@1.6.0` installed via npm in the GitHub Actions runner (pinned to match the version this environment already uses elsewhere).
- Amends this repo's contributor workflow: finishing an OpenSpec change (`openspec archive --yes`, done in the same PR before merge per this repo's existing process) must be followed by re-running `make projected-specs` and committing the resulting removal.
