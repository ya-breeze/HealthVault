#!/usr/bin/env bash
set -euo pipefail

BASE="${1:-origin/main}"

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

PROJECTED_DIR="openspec/specs.projected"
README_PATH="openspec/specs.projected.README.md"
DISCOVERY_PATHSPEC=':(glob)openspec/changes/*/specs/**'

generated_header() {
	cat <<EOF
<!-- GENERATED FILE — DO NOT EDIT.
     Regenerate with: make projected-specs
     See ${README_PATH} for details. -->

EOF
}

# 1.1 Discover touched change ids by diffing committed refs. The :(glob)
# pathspec magic is required: git's default (non-glob) pathspec lets '*'
# match across '/', which would falsely match an archived-and-committed
# delta under openspec/changes/archive/<date>-<id>/specs/**.
mapfile -t CHANGED_FILES < <(git diff --name-only "$BASE"...HEAD -- "$DISCOVERY_PATHSPEC")

CHANGE_IDS=()
if [[ ${#CHANGED_FILES[@]} -gt 0 ]]; then
	mapfile -t CHANGE_IDS < <(
		for f in "${CHANGED_FILES[@]}"; do
			id="${f#openspec/changes/}"
			id="${id%%/*}"
			printf '%s\n' "$id"
		done | sort -u
	)
fi

# 1.2 Zero-touched-ids path.
if [[ ${#CHANGE_IDS[@]} -eq 0 ]]; then
	rm -rf "$PROJECTED_DIR"
	exit 0
fi

# 1.6 Derive the touched-capability set from the touched delta paths.
mapfile -t CAPABILITIES < <(
	for f in "${CHANGED_FILES[@]}"; do
		rest="${f#*/specs/}"
		cap="${rest%%/*}"
		printf '%s\n' "$cap"
	done | sort -u
)

# 1.3 Pre-archive cross-change conflict check, before any worktree is
# created. Parses every touched change id's full delta file set (not just
# the files changed in this diff) at committed HEAD, so archiving a change
# whose conflicting file wasn't itself touched in this diff is still caught.
declare -A PAIR_OWNER
CONFLICT=0

record_pair() {
	local cap="$1" name="$2" id="$3" key
	key="${cap}"$'\x1f'"${name}"
	if [[ -n "${PAIR_OWNER[$key]:-}" && "${PAIR_OWNER[$key]}" != "$id" ]]; then
		echo "ERROR: conflicting touched changes '${PAIR_OWNER[$key]}' and '$id' both touch requirement '$name' in capability '$cap'" >&2
		CONFLICT=1
	else
		PAIR_OWNER[$key]="$id"
	fi
}

for id in "${CHANGE_IDS[@]}"; do
	mapfile -t DELTA_FILES < <(git ls-tree -r --name-only HEAD -- "openspec/changes/$id/specs" | grep '/spec\.md$' || true)
	for df in "${DELTA_FILES[@]}"; do
		[[ -z "$df" ]] && continue
		rest="${df#*/specs/}"
		cap="${rest%%/*}"
		content="$(git show "HEAD:$df")"

		while IFS= read -r name; do
			[[ -z "$name" ]] && continue
			record_pair "$cap" "$name" "$id"
		done < <(printf '%s\n' "$content" | grep -oP '^### Requirement: \K.*' || true)

		# shellcheck disable=SC2016 # backticks are literal regex chars, not shell expansion
		while IFS= read -r name; do
			[[ -z "$name" ]] && continue
			record_pair "$cap" "$name" "$id"
		done < <(printf '%s\n' "$content" | grep -oP '^- FROM: `### Requirement: \K[^`]*' || true)

		# shellcheck disable=SC2016 # backticks are literal regex chars, not shell expansion
		while IFS= read -r name; do
			[[ -z "$name" ]] && continue
			record_pair "$cap" "$name" "$id"
		done < <(printf '%s\n' "$content" | grep -oP '^- TO: `### Requirement: \K[^`]*' || true)

		# OpenSpec 1.6.0 also accepts REMOVED requirements written as a bullet
		# (backticks optional): "- `### Requirement: <name>`". Mirrors the
		# CLI's own parser regex (requirement-blocks.js: parseRemovedNames)
		# so this preflight rejects the same conflicts the CLI would silently
		# let a later archive overwrite.
		# shellcheck disable=SC2016 # backticks are literal regex chars, not shell expansion
		while IFS= read -r name; do
			[[ -z "$name" ]] && continue
			record_pair "$cap" "$name" "$id"
		done < <(printf '%s\n' "$content" | grep -oP '^\s*-\s*`?###\s*Requirement:\s*\K.*?(?=`?\s*$)' || true)
	done
done

if [[ "$CONFLICT" -eq 1 ]]; then
	exit 1
fi

# 1.4 Isolated, disposable worktree checked out from committed HEAD.
WORKTREE_DIR="$(mktemp -d)"
cleanup() {
	git worktree remove --force "$WORKTREE_DIR" >/dev/null 2>&1 || rm -rf "$WORKTREE_DIR"
	git worktree prune >/dev/null 2>&1 || true
}
trap cleanup EXIT

git worktree add --detach "$WORKTREE_DIR" HEAD >/dev/null

# 1.5 Archive each touched change id, in sorted order, inside the worktree.
for id in "${CHANGE_IDS[@]}"; do
	if ! (cd "$WORKTREE_DIR" && openspec archive "$id" --yes); then
		echo "ERROR: 'openspec archive $id --yes' failed; openspec/specs.projected/ left untouched" >&2
		exit 1
	fi
done

# 1.7 Rebuild the projection from scratch, scoped to touched capabilities only.
rm -rf "$PROJECTED_DIR"
mkdir -p "$PROJECTED_DIR"

# 1.8 / 1.9 Write final text + deterministic diff, each with a generated-file header.
for cap in "${CAPABILITIES[@]}"; do
	mkdir -p "$PROJECTED_DIR/$cap"

	before="openspec/specs/$cap/spec.md"
	after="$WORKTREE_DIR/openspec/specs/$cap/spec.md"

	if [[ ! -f "$after" ]]; then
		echo "ERROR: expected 'openspec archive' to write $after for touched capability '$cap', but it does not exist; openspec/specs.projected/ may be incomplete" >&2
		exit 1
	fi

	# Read the "before" side from committed HEAD, not the primary working
	# tree: an uncommitted local edit to openspec/specs/$cap/spec.md must
	# not change spec.diff, since "after" is already computed purely from
	# HEAD via the worktree.
	before_tmp=""
	before_input="/dev/null"
	if git cat-file -e "HEAD:$before" 2>/dev/null; then
		before_tmp="$(mktemp)"
		git show "HEAD:$before" >"$before_tmp"
		before_input="$before_tmp"
	fi

	{
		generated_header
		cat "$after"
	} >"$PROJECTED_DIR/$cap/spec.md"

	{
		generated_header
		diff -u --label "openspec/specs/$cap/spec.md" --label "openspec/specs/$cap/spec.md" "$before_input" "$after" || true
	} >"$PROJECTED_DIR/$cap/spec.diff"

	if [[ -n "$before_tmp" ]]; then
		rm -f "$before_tmp"
	fi
done
