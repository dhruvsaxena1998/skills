# Playbook

This repo holds agent skills taken from several upstreams, the local edits made on top of
them, and `skillset`, the Go binary that links them into agent folders. `README.md`
explains what the commands do. This file is what to do when you change the repo.

Work on a branch. Run `skillset` commands from the clone `skillset init` recorded: `sync`
reads the repo path out of `~/.config/skillset/config.json`, not the working directory.

## Add a skill from a source we already have

1. Add the folder name to that source's `skills` list in `sources.json`.
2. Add it to the same source's `deps` map (see below).
3. Commit, then `skillset sync <source>`. Sync refuses to run with a dirty tree.
4. `skillset enable <skill>` then `skillset apply`, so it works on this machine.

Removing a name from the list deletes the skill on the next sync.

## Add a source

Add an entry to `sources.json`:

```json
"name": {
  "repo": "https://github.com/owner/repo",
  "ref": "main",
  "path": "path/to/skills",
  "skills": ["skill-a", "skill-b"],
  "files": ["hooks/some-hook-*.js"],
  "deps": {
    "skill-a": {},
    "skill-b": { "requires": ["skill-a"], "optional": ["unslop"] }
  }
}
```

Skills are found by folder name anywhere under `path`, so upstream regrouping its folders
doesn't break the sync. `files` is optional: globs of non-skill files copied to the same
relative path, e.g. ponytail's hook scripts. Sync fails if a glob matches nothing.

Then `skillset sync name`. The first run creates the orphan `vendor/name` branch.

## Add a skill of your own

It goes straight into `skills/<name>/SKILL.md` on the branch, with no `sources.json`
entry. It is not under any source, so sync never touches it, and the selection screen
lists it under `NOT OURS` only if something outside the repo put it in an agent folder.
Enable it here as above.

## Patch an upstream skill

Edit the file in `skills/<name>/` and commit it on its own, with a message that starts
`patch`:

```
patch(<skill>): <what changed>
```

The vendor branches are never edited by hand. A patch commit lives on the branch sync
merges into, so git's 3-way merge keeps it wherever upstream didn't touch the same lines.
`git log --oneline --grep '^patch'` lists them, and the table in `README.md` says why each
one exists — add a row when you add a patch.

## Dependencies

Every skill in a `skills` list gets an entry in that source's `deps` map, `{}` when it
needs nothing. `requires` is what the skill can't work without; `optional` is what
improves it. Both may name skills from another source.

- Enabling a skill enables its `requires`, transitively.
- Disabling one disables every skill that requires it, transitively.
- A disabled `optional` only prints a warning.
- A skill whose `requires` isn't in the repo can't be enabled at all.

Skills reference each other by relative path (`../unslop/SKILL.md`), not by name, because
most pstack skills set `disable-model-invocation` and can't be called through the Skill
tool. `skillset sync` warns when a skill links to a sibling it doesn't list in `deps`.
Fix the map when you see that warning.

## Hooks

A skill that ships a hook needs two things: the script vendored through the source's
`files` globs, and an entry in `hooks/hooks.json` naming the skill it belongs to:

```json
"SessionStart": [
  { "skill": "ponytail", "hooks": [{ "type": "command", "command": "node \"{{ROOT}}/hooks/<script>.js\"" }] }
]
```

`{{ROOT}}` expands to the repo path with forward slashes. `skillset apply` registers the
entry only while that skill is enabled, and removes it when it isn't.

## Finish

```sh
skillset sync <source>      # if a source changed
skillset enable <skill>     # on this machine
skillset apply
git push -u origin <branch>
```

Everything in `~/.config/skillset/` is this machine's own selection. It is outside the
repo; never add it to a commit.
