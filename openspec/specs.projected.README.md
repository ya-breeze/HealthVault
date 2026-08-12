# `openspec/specs.projected/`

## What this is

OpenSpec delta specs under `openspec/changes/<id>/specs/<capability>/spec.md` use
`## ADDED/MODIFIED/REMOVED/RENAMED Requirements` sections. A `MODIFIED` requirement
records only its new text — not what it replaces — and the delta file itself is new in
git, so a plain PR diff shows isolated new prose with no old-to-new comparison. Deltas
only merge into the canonical `openspec/specs/` tree at `openspec archive`, which in
this repo's workflow happens right before merge — too late for that comparison to help
review.

`openspec/specs.projected/<capability>/` is a **generated preview** of what
`openspec/specs/<capability>/spec.md` will read after the current branch's touched
changes are archived. For each capability touched by an open change on the branch, it
contains:

- `spec.md` — the full post-archive requirement text, for reading the final wording as
  a coherent whole.
- `spec.diff` — a unified diff between the current `openspec/specs/<capability>/spec.md`
  and that post-archive text, so the change is explicit without depending on git having
  a prior commit to diff against.

**Review the projected file for final wording, and the delta under
`openspec/changes/<id>/specs/` for intent and rationale.**

## It is generated — never hand-edit it

Every file under `openspec/specs.projected/` starts with a header saying so. The
directory is rebuilt from scratch on every run of the generator (`rm -rf` then
recreate), so any hand edit is silently lost the next time someone regenerates it. CI
also rejects a PR whose committed `openspec/specs.projected/` doesn't match what
regenerating it produces.

To regenerate:

```
make projected-specs
```

## The empty-on-main lifecycle invariant

`openspec/specs.projected/` exists **only** while a branch has open (unarchived, and
not yet committed-as-archived) OpenSpec deltas it touches. It is never a permanent
mirror of `openspec/specs/`. Once every touched change on a branch has been archived
and that archive is committed, regenerating produces no output at all, and
`openspec/specs.projected/` does not exist. `main` never carries `specs.projected/`
content.

## Finishing a change: two commits, not one

Both discovery (which change ids are "touched") and the generator's isolated worktree
operate on **committed** `HEAD`, not the working tree. That means running
`openspec archive <id> --yes` and `make projected-specs` back-to-back, before
committing the archive, does **not** produce the empty state — the generator still
sees the pre-archive delta and regenerates the same non-empty projection. Finishing a
change requires two separate commits, in this order:

1. Run `openspec archive <id> --yes`, then commit that move on its own.
2. Only then run `make projected-specs` — it now finds zero touched changes and
   removes `openspec/specs.projected/` — and commit that removal as a second commit.

Both commits land in the same PR, before merge, per this repo's existing OpenSpec
workflow.
