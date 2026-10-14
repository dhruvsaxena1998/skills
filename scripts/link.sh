#!/usr/bin/env bash
# Symlink every skills/<name> into each agent's skills directory.
# Links owned by something else are left alone. `--unlink` removes only links pointing into this repo.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
targets=("$HOME/.claude/skills" "$HOME/.agents/skills" "$HOME/.pi/agent/skills")
unlink=$([[ ${1:-} == --unlink ]] && echo 1 || true)

for t in "${targets[@]}"; do
  mkdir -p "$t"
  for l in "$t"/*; do
    [[ -L $l ]] || continue
    dest=$(readlink "$l")
    [[ $dest == "$root/skills/"* ]] || continue
    # remove all ours on --unlink, otherwise only links to skills that were deleted
    if [[ -n $unlink || ! -f $dest/SKILL.md ]]; then rm "$l"; echo "removed $l"; fi
  done
  [[ -n $unlink ]] && continue

  for s in "$root"/skills/*/; do
    s=${s%/}
    [[ -f $s/SKILL.md ]] || continue
    l="$t/$(basename "$s")"
    if [[ -L $l && $(readlink "$l") == "$s" ]]; then continue
    elif [[ -e $l || -L $l ]]; then echo "skip $l: exists and isn't from this repo" >&2
    else ln -s "$s" "$l"; echo "linked $l"; fi
  done
done

# Register hooks/hooks.json in each agent's hook config. Ours are appended after existing
# entries (Codex keys hook trust by position) and found again by their path into this repo.
hook_files=("$HOME/.claude/settings.json:.hooks" "$HOME/.codex/hooks.json:.hooks")
ours=$(sed "s|{{ROOT}}|$root|g" "$root/hooks/hooks.json")
for spec in "${hook_files[@]}"; do
  file=${spec%%:*}
  [[ -f $file ]] || { echo "skip hooks: $file not found" >&2; continue; }
  # shellcheck disable=SC2016
  updated=$(jq --arg root "$root/hooks/" --argjson ours "$ours" --arg unlink "$unlink" '
    def mine: any(.hooks[]?; .command | contains($root));
    .hooks = (reduce (((.hooks // {}) | keys) + ($ours | keys) | unique)[] as $e (.hooks // {};
      .[$e] = ([(.[$e] // [])[] | select(mine | not)] + (if $unlink == "" then ($ours[$e] // []) else [] end))
      | if .[$e] == [] then del(.[$e]) else . end))
  ' "$file")
  if [[ $updated != "$(jq . "$file")" ]]; then
    cp "$file" "$file.bak"
    printf '%s\n' "$updated" > "$file"
    echo "hooks updated in $file (backup: $file.bak)"
  fi
done
