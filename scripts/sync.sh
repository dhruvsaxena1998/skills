#!/usr/bin/env bash
# Snapshot a source's selected skills from upstream onto its vendor/<source> branch,
# then merge that branch into the current one so local edits are kept by a 3-way merge.
set -euo pipefail

src=${1:?usage: scripts/sync.sh <source>}
root=$(git rev-parse --show-toplevel)
cfg=$(jq -e --arg s "$src" '.[$s]' "$root/sources.json") || { echo "unknown source: $src" >&2; exit 1; }
repo=$(jq -r .repo <<<"$cfg")
ref=$(jq -r .ref <<<"$cfg")
path=$(jq -r .path <<<"$cfg")
branch="vendor/$src"

git -C "$root" diff --quiet HEAD || { echo "commit or stash your changes first" >&2; exit 1; }

tmp=$(mktemp -d)
cleanup() { git -C "$root" worktree remove --force "$tmp/vendor" 2>/dev/null || true; rm -rf "$tmp"; }
trap cleanup EXIT

git clone -q --depth 1 --branch "$ref" "$repo" "$tmp/up"
sha=$(git -C "$tmp/up" rev-parse --short HEAD)

if git -C "$root" rev-parse -q --verify "refs/heads/$branch" >/dev/null; then
  git -C "$root" worktree add -q "$tmp/vendor" "$branch"
else
  # orphan so the vendor branch only ever holds this source's files
  git -C "$root" worktree add -q --orphan -b "$branch" "$tmp/vendor"
fi

rm -rf "$tmp/vendor/skills" "$tmp/vendor/licenses"
mkdir -p "$tmp/vendor/skills" "$tmp/vendor/licenses"
for name in $(jq -r '.skills[]' <<<"$cfg"); do
  # look skills up by name so upstream can regroup folders without breaking us
  matches=$(find "$tmp/up/$path" -type d -name "$name" -exec test -f '{}/SKILL.md' \; -print)
  [[ $(grep -c . <<<"$matches") == 1 ]] || { echo "expected one '$name' in $repo/$path, found: ${matches:-none}" >&2; exit 1; }
  cp -R "$matches" "$tmp/vendor/skills/$name"
done
[[ -f $tmp/up/LICENSE ]] && cp "$tmp/up/LICENSE" "$tmp/vendor/licenses/$src-LICENSE"

git -C "$tmp/vendor" add -A
if git -C "$tmp/vendor" diff --cached --quiet; then
  echo "$branch already matches $repo@$sha"
else
  git -C "$tmp/vendor" commit -q -m "sync $src @ $sha" -m "$repo"
  echo "committed $branch: $src @ $sha"
fi

git -C "$root" merge --no-edit --allow-unrelated-histories "$branch" -m "merge $branch ($sha)" \
  || { echo "resolve the conflicts above, then: git commit" >&2; exit 1; }
