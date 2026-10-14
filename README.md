# Skills

The agent skills I use, collected from several upstream repos, with room for my own edits.

## Layout

```text
skills/<name>/SKILL.md   every skill, flat, whatever its source
sources.json             which skills to take from which upstream
licenses/                upstream licenses
scripts/sync.sh          pull upstream skills and merge them in
scripts/link.sh          symlink skills into agent directories
```

## Sources

| Source       | Upstream                                                                 | Branch              |
| ------------ | ------------------------------------------------------------------------ | ------------------- |
| `mattpocock` | [mattpocock/skills](https://github.com/mattpocock/skills)                | `vendor/mattpocock` |

## How upstream sync works

Each source has a `vendor/<source>` branch that holds only that source's skills, exactly as upstream ships them. It is never edited by hand. `main` merges those branches and carries my edits as ordinary commits on top.

```sh
scripts/sync.sh mattpocock
```

This clones upstream, replaces the selected skills on `vendor/mattpocock`, commits `sync mattpocock @ <sha>`, and merges that branch into the current one. Git's 3-way merge keeps my edits wherever upstream didn't touch the same lines; anything else shows up as a normal merge conflict.

See what I've changed relative to upstream:

```sh
git diff vendor/mattpocock main -- skills/tdd
```

## Add or remove a skill

Edit the source's `skills` list in `sources.json`, commit, then run `scripts/sync.sh <source>`. Skills are found by folder name anywhere under the source's `path`, so upstream regrouping its folders doesn't break the sync. Removing a skill from the list deletes it on the next sync.

## Add a source

Add an entry to `sources.json`:

```json
"name": {
  "repo": "https://github.com/owner/repo",
  "ref": "main",
  "path": "path/to/skills",
  "skills": ["skill-a", "skill-b"]
}
```

Then run `scripts/sync.sh name`. The first run creates `vendor/name`.

Skills of my own go straight into `skills/<name>/` on `main`.

## Install

```sh
scripts/link.sh            # symlink skills into ~/.claude, ~/.agents and ~/.pi/agent
scripts/link.sh --unlink   # remove only the links that point into this repo
```

Links are symlinks, so edits in this repo take effect immediately. Existing entries that don't come from this repo are left alone. Re-run after adding or removing skills.

Requires `git` 2.42+ and `jq`.
