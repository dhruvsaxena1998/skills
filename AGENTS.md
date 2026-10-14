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

A skill you write goes straight into `skills/<name>/` on the branch, with no
`sources.json` entry. Sync never touches it, and the selection screen lists it under
`unlisted`, the group that holds every skill in the repo no source claims.

`isSkill()` is the whole test for what counts as a skill: `skills/<name>/SKILL.md` has to
exist. Nothing else is checked. The folder name is the skill's name everywhere — in
`skillset enable`, in the `deps` map, in the link in the agent folder, in the
`../<name>/SKILL.md` paths other skills use. All 29 skills here are lowercase with
hyphens and repeat their folder name in the frontmatter `name`. Match that. The folder is
linked whole, so references, scripts and subagent files sit beside `SKILL.md` and reach
the agent with it, the way `skills/tdd/mocking.md`, `skills/diagnosing-bugs/scripts/` and
`skills/research/agents/` do.

1. Write `SKILL.md`. Every skill here sets `name` and `description`; 13 also set
   `disable-model-invocation`.

   ```yaml
   ---
   name: release-notes
   description: Write release notes from a tag range. Use when the user asks for release notes or a changelog for a version.
   ---
   ```

   `description` is the only frontmatter field `skillset` reads. `descriptionOf()` takes
   the rest of the line, or, when the line ends in `>`, `|`, `>-` or `|-`, the indented
   lines under it joined into one. A plain value wrapped onto a second line without one of
   those markers loses everything after the first line. The description is what the skill
   list and the detail pane show, and what an agent reads to decide whether to run the
   skill, so say what the skill does and when to use it. Set
   `disable-model-invocation: true` only if the skill must be asked for by name and never
   chosen by the agent; skills that set it are reached by relative path instead, as
   Dependencies below explains.

2. Declare dependencies, if it has any. The `deps` map exists only inside a source entry,
   and a skill of your own is in no source. `loadDeps()` merges every source's map into
   one before any name is looked up, and nothing checks that a key in a `deps` map is in
   that source's `skills` list. So your skill can declare `requires` and `optional`, but
   only by putting its entry in some source's map — there is no repo-level `deps`. Put it
   in the map of the source that ships the skill it depends on:

   ```json
   "pstack": {
     "skills": ["unslop"],
     "deps": {
       "unslop": {},
       "release-notes": { "requires": ["unslop"] }
     }
   }
   ```

   Enabling, the disable cascade and the optional warning all work from there. If the
   skill depends on nothing, write nothing: a name absent from the map requires nothing
   and enables on its own. Two things follow from the entry living under a source that
   doesn't ship the skill. `reportDrift()` only walks the skills in a source's `skills`
   list, so sync never warns you that your skill links a sibling it doesn't declare —
   check that yourself. And the entry reads as if it belonged to that source, so keep it
   next to the skill it requires, where the reason is visible.

3. Register a hook, if it ships one. Same two pieces as a vendored hook, minus the
   vendoring: commit the script into `hooks/` yourself, because the `files` globs only
   copy files out of a source's upstream and your skill has no upstream, then add an entry
   to `hooks/hooks.json` naming the skill. The Hooks section below has the shape and what
   `{{ROOT}}` expands to. `repoHooks()` fails on any entry without a `skill` field, which breaks
   `apply` for every skill, not just yours.

4. Turn it on here.

   ```sh
   skillset enable release-notes
   skillset apply
   ```

   `enable` writes the name into both `enabled` and `seen` in
   `~/.config/skillset/config.json`; `apply` creates the links. `seen` is how "new" works:
   `init` fills it with every skill in the repo, and the screen marks a skill missing from
   it as new and leaves it off. Because `enable` adds the name to `seen` too, the skill
   you just wrote is on here and never announced as new here, while on the other machine
   it arrives unseen, marked new, and off until it is turned on there.

   Both commands read the clone path out of `~/.config/skillset/config.json`, not the
   working directory. Write the skill in a second clone or a worktree and `enable` answers
   `no skill named "release-notes"` until that branch is checked out in the recorded
   clone.

5. Check it. That `apply` prints `linked <home>/.claude/skills/release-notes` once per
   enabled target.

   ```sh
   ls -l ~/.claude/skills/release-notes
   skillset apply
   ```

   The link points into the clone, and the second `apply` prints nothing for the skill.
   `skip release-notes: not in the repo` means `skills/release-notes/SKILL.md` isn't under
   the recorded clone. Then open `skillset`: the skill is under `unlisted`, checked, with
   no `new` badge, and the middle column shows your description. An empty description
   there means `descriptionOf()` found no `description:` to read.

6. Commit the skill, and the `sources.json` and `hooks/hooks.json` edits with it:

   ```
   feat(<skill>): <what it does>
   ```

   Not `patch(...)`. That prefix is only for edits to a skill that came from upstream:
   `patchesFor()` greps `^patch` to list a skill's local patches and the table in
   `README.md` says why each one exists. A skill of your own has no upstream to differ
   from. `~/.config/skillset/` holds this machine's selection and stays out of the commit.

End to end, the `release-notes` example is three files and four commands:

```text
skills/release-notes/SKILL.md     the skill
skills/release-notes/FORMAT.md    a reference it links as ./FORMAT.md
sources.json                      "release-notes": { "requires": ["unslop"] } in pstack's deps
```

```sh
git add skills/release-notes sources.json
git commit -m "feat(release-notes): write release notes from a tag range"
skillset enable release-notes
skillset apply
```

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
improves it. Both may name skills from another source. A skill of your own is in no
`skills` list, so its entry goes where Add a skill of your own says.

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
