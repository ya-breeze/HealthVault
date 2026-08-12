## ADDED Requirements

### Requirement: Projected spec generation from touched changes
The generator SHALL determine which OpenSpec change ids are touched on the current branch by diffing `openspec/changes/*/specs/**` against a base ref (default `origin/main`), extracting the change id from each changed path's first segment after `openspec/changes/`. For each touched id, the generator SHALL run `openspec archive <id> --yes` inside an isolated worktree and SHALL rebuild `openspec/specs.projected/` from the resulting `openspec/specs/` tree in that worktree.

#### Scenario: Single touched change
- **WHEN** the branch's diff against the base ref includes files under `openspec/changes/add-foo/specs/**` and no other change's `specs/**` paths
- **THEN** the generator archives only `add-foo` in the isolated worktree and writes the resulting `openspec/specs/` tree to `openspec/specs.projected/`

#### Scenario: Multiple touched changes in one PR
- **WHEN** the branch's diff against the base ref includes files under both `openspec/changes/add-foo/specs/**` and `openspec/changes/add-bar/specs/**`
- **THEN** the generator archives `add-foo` and `add-bar` together, in sorted order, inside the same isolated worktree, and `openspec/specs.projected/` reflects both changes merged on top of the base specs

#### Scenario: Untouched capabilities are unaffected
- **WHEN** a touched change modifies only capability `foo`, and capability `bar` exists in the base specs unchanged
- **THEN** the regenerated `openspec/specs.projected/bar/` is byte-identical to the base `openspec/specs/bar/`, producing no diff against a previously committed projection for `bar`

### Requirement: Zero-change case leaves no projection
When no change ids are touched relative to the base ref, the generator SHALL remove `openspec/specs.projected/` entirely (if present) and exit successfully, without creating a worktree or invoking `openspec archive`.

#### Scenario: PR with no OpenSpec delta changes
- **WHEN** the branch's diff against the base ref contains no paths matching `openspec/changes/*/specs/**`
- **THEN** the generator exits 0, and `openspec/specs.projected/` does not exist afterward

#### Scenario: Self-cleaning after a change is archived
- **WHEN** a branch previously had a committed, non-empty `openspec/specs.projected/` for change `add-foo`, and `add-foo` has since been archived for real (moving its delta out of `openspec/changes/add-foo/specs/**` into the archive folder)
- **THEN** re-running the generator finds zero touched change ids and removes `openspec/specs.projected/`, leaving no projected content for `add-foo` to carry into a merge

### Requirement: Generation is isolated from the primary working tree
The generator SHALL perform all `openspec archive` calls inside a disposable, detached git worktree checked out from `HEAD`, and SHALL NOT modify any file in the primary working tree other than `openspec/specs.projected/`. The worktree SHALL be removed on exit, including on failure.

#### Scenario: Safe to run with uncommitted changes elsewhere
- **WHEN** the primary working tree has uncommitted edits to files outside `openspec/`
- **THEN** running the generator does not alter, stage, or discard those uncommitted edits

#### Scenario: Worktree removed after a successful run
- **WHEN** the generator completes successfully
- **THEN** no worktree registered by the run remains (`git worktree list` shows none of the run's temporary paths)

#### Scenario: Worktree removed after a failed run
- **WHEN** the generator aborts due to an `openspec archive` failure
- **THEN** no worktree registered by the run remains, despite the failure

### Requirement: Archive failure aborts without partial changes
If `openspec archive <id> --yes` fails for any touched change id inside the worktree, the generator SHALL exit non-zero, SHALL report the failure, and SHALL leave `openspec/specs.projected/` in the primary working tree exactly as it was before the run.

#### Scenario: Invalid delta causes archive to fail
- **WHEN** a touched change's delta spec fails `openspec archive`'s validation (e.g. a malformed requirement)
- **THEN** the generator exits non-zero, prints the underlying `openspec archive` error, and does not write, remove, or partially update `openspec/specs.projected/`

### Requirement: Generated files are marked non-editable
Every markdown file written into `openspec/specs.projected/` SHALL begin with a header comment identifying the file as generated and naming the command to regenerate it.

#### Scenario: Header present on every projected file
- **WHEN** the generator writes any `openspec/specs.projected/<capability>/spec.md`
- **THEN** the file's first line is a comment stating it is generated, must not be hand-edited, and naming `make projected-specs` as the regeneration command

### Requirement: CI gate enforces regenerated output matches committed output
A CI workflow SHALL run on pull requests that touch `openspec/**`, regenerate `openspec/specs.projected/` against the PR's base branch, and fail the check if the regenerated content differs from what is committed in the PR, with an error naming the exact local command to run.

#### Scenario: Stale projection fails CI
- **WHEN** a PR touching `openspec/changes/*/specs/**` is opened without a matching regeneration of `openspec/specs.projected/`
- **THEN** the CI job fails and its output names `make projected-specs` as the command to run and commit

#### Scenario: Up-to-date projection passes CI
- **WHEN** a PR's committed `openspec/specs.projected/` matches what regenerating it against the PR's base branch produces
- **THEN** the CI job succeeds

### Requirement: Projected specs never persist past merge
Because `main` always reaches a state where every touched change has been archived (this repo's workflow requires archiving in the same PR before merge), `openspec/specs.projected/` SHALL NOT contain committed content on `main`.

#### Scenario: Archived change results in empty projection before merge
- **WHEN** a PR's change has been archived for real as the final step before merge, per this repo's workflow
- **THEN** regenerating the projection at that point produces no content, and the PR's final committed state has no `openspec/specs.projected/` directory

### Requirement: Task runner integration
Running `make projected-specs` SHALL invoke the generator script against the default base ref, applying all of the above behavior.

#### Scenario: Make target invokes the generator
- **WHEN** a contributor runs `make projected-specs`
- **THEN** the generator script runs exactly as it would when invoked directly, updating or removing `openspec/specs.projected/` per the current branch's touched changes
