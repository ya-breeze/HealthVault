# Retire OpenSpec from HealthVault

Idea: ya-breeze/idea-forge#33

## Why

OpenSpec was retired across this environment on 2026-08-26. `CLAUDE.md` no longer asks for a change
under `openspec/changes/` before writing code (ya-breeze/Useful#33), the pipeline that drove those
changes was rebuilt around a single per-change file (ya-breeze/idea-forge#30), and the CLI is gone
from the container bootstrap. This repository still carries the tree those rules produced.

What was wrong with `openspec/specs/` was never its volume — it was that the tree *claimed to be
current*. A canonical spec asserts "this is how the system behaves now", so every merge that did
not update it made the tree a lie, and keeping that assertion true was the entire ceremony: spec
deltas, `openspec validate --strict`, exact header matching, projected specs, the two-commit
archive. A per-change spec frozen at merge asserts only "this is what we decided, then" — a
historical document by construction, which nothing has to maintain.

The tree here is 233 files: 30 capability specs, 32 archived changes.

## How

`openspec/` is deleted outright. Nothing is migrated: the specs describe behaviour that the code
already implements and the tests already cover, so re-typing them into `docs/specs/` would recreate
the same maintenance claim under a new path. Anything genuinely worth reading survives in git
history, where it stays reachable without asserting anything about the present.

`openspec/` was wired into the build here, unlike the other repos. Four things go with it: the
`openspec-projected-specs` GitHub Actions workflow, which installed the CLI on every PR and failed
the build when `openspec/specs.projected/` was stale; `scripts/generate-projected-specs.sh`, which
that workflow and the `projected-specs` make target both called; the make target itself and its
`.PHONY` entry; and `.gitattributes`, whose only line marked the projected tree as generated.

Deliberately left: **115 code comments across 40 files cite `openspec/specs/...` paths** — things
like `// See openspec/specs/data-model "Food logging tables"`. They now point at files that exist
only in git history. Rewriting all 115 would touch far more code than this change is worth, and
each one still records real provenance for why the code around it is shaped the way it is. This is
a known, deliberate state, not an oversight.

## Validation Commands

- `make lint`

`make lint` is the gate because this diff edits the `Makefile`: it proves the file still parses and
the remaining targets still run after `projected-specs` was cut out.

### Task 1: Delete the tree

- [x] Remove `openspec/` in full
- [x] Confirm nothing outside it still refers to the deleted paths, beyond what is named above
- [x] Mark completed

### Task 2: Unwire it from the build

- [x] Delete the `openspec-projected-specs` workflow and `scripts/generate-projected-specs.sh`
- [x] Drop the `projected-specs` target and its `.PHONY` entry from the `Makefile`
- [x] Delete `.gitattributes`, whose only line described the projected tree
- [x] Confirm `make lint` still passes
- [x] Mark completed
