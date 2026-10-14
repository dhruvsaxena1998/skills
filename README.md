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
| `pstack`     | [cursor/plugins › pstack](https://github.com/cursor/plugins/tree/main/pstack) | `vendor/pstack`     |

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

## Patches

My edits to upstream skills, each in its own `patch` commit. List them with `git log --oneline --grep '^patch'`.

| Skill | Change | Why |
| ----- | ------ | --- |
| `unslop` | Dropped `disable-model-invocation`; the description says when to apply it | Agents run it on their own when writing prose |
| `pr` | Prose goes through `unslop`; **Blast Radius** is filled by following `blast-radius` | Combine Matt's PR template with pstack's writing and risk checks |
| `blast-radius` | Removed `arena`/`how`/`why` steps; pulls PRs with `gh` or MRs with `glab` | Those pstack skills only work in Cursor; support GitHub and GitLab |
| `to-spec`, `to-tickets`, `to-questionnaire`, `research`, `domain-modeling`, `handoff`, `technical-writing` | Write their output through `unslop` | Consistent, plain prose in every artifact |
| `typescript-best-practices` | Dropped `disable-model-invocation`; principles linked by path | Applies automatically when editing `.ts` |
| `tdd` | Added "When not to add a test" from pstack's `tdd` | Prefer no test over a bad test |

Skills reference each other by relative path (`../unslop/SKILL.md`), not by name, because most pstack skills set `disable-model-invocation` and can't be called through the Skill tool.

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
