# Playbook

This repo holds agent skills taken from several upstreams, the local edits made on top of
them, and `skillset`, the Go binary that links them into agent folders. `README.md`
explains what the commands do, and so does `skillset help` where there is no README. This
file is what to do when you change the repo.

Starting from an empty folder instead: sources.json below has the two files a repo needs
and the four commands that make one. Add this file with `skillset playbook > AGENTS.md`
and its one-line `CLAUDE.md` with `echo '@AGENTS.md' > CLAUDE.md`, and every recipe here
applies to that repo as written.

Work on a branch. Run `skillset` commands from the clone `skillset init` recorded: `sync`
reads the repo path out of `~/.config/skillset/config.json`, not the working directory. In a
clone that has never synced, `git fetch origin 'refs/heads/vendor/*:refs/heads/vendor/*'`
before the first `sync`, or it rebuilds the vendor branch as an orphan and every patched
file conflicts.

## Add a skill from a source we already have

1. Add the folder name to that source's `skills` list in `sources.json`.
2. Add it to the same source's `deps` map (see below).
3. Commit, then `skillset sync <source>`. Sync refuses to run with a dirty tree.
4. `skillset enable <skill>` then `skillset apply`, so it works on this machine.

Removing a name from the list deletes the skill on the next sync.

## Add a source

Add an entry to `sources.json`, with the fields below, then `skillset sync <name>`. The
first run creates the orphan `vendor/<name>` branch.

## sources.json

The file is one object, a source name to its source. The name is what
`skillset sync <name>` takes, the branch it keeps (`vendor/<name>`), the license it writes
(`licenses/<name>-LICENSE`), and the group the screen lists those skills under. A skill in
`skills/` that no source claims is `unlisted`.

| Field    | Type     | Required | What it is                                                                   |
| -------- | -------- | -------- | ---------------------------------------------------------------------------- |
| `repo`   | string   | yes      | What `git clone` is handed: any URL git can clone, or a local path           |
| `ref`    | string   | yes      | The branch or tag to clone. No default, and an empty one fails the clone     |
| `path`   | string   | no       | The folder under the clone to search. Omitted means the clone root           |
| `skills` | string[] | yes      | The folder names to take, found anywhere under `path`                        |
| `files`  | string[] | no       | Globs of other files to copy, relative to the clone root, not to `path`      |
| `deps`   | object   | no       | A skill name to `{ "requires", "optional", "agents" }`, see Dependencies     |

Paths in `path` and `files` use forward slashes on every OS.

Every field, in ponytail's own entry with its skill list cut to two:

```json
{
  "ponytail": {
    "repo": "https://github.com/dietrichgebert/ponytail",
    "ref": "main",
    "path": "skills",
    "skills": ["ponytail", "ponytail-debt"],
    "files": [
      "hooks/ponytail-*.js",
      "hooks/ponytail-statusline.sh",
      "hooks/ponytail-statusline.ps1"
    ],
    "deps": {
      "ponytail": {},
      "ponytail-debt": { "requires": ["ponytail"] }
    }
  }
}
```

What sync does with them, and where each one bites:

- A skill is found by folder name anywhere under `path`, so upstream regrouping its folders
  doesn't break the sync. Exactly one folder must match and it must hold a `SKILL.md`; none
  or two both fail the sync by name.
- `files` copies each match to the same relative path here, which is how a skill's hook
  scripts arrive. A glob matching nothing fails the sync, so a renamed upstream file is
  caught rather than silently dropped.
- Those globs are Go's `filepath.Glob`: `*` stops at a `/` and `**` is not recursive.
  `hooks/**/*.js` means exactly one folder deep and silently skips `hooks/x.js`. Write one
  entry per level.
- The nearest LICENSE is copied to `licenses/<name>-LICENSE`: `<path>/LICENSE`, then its
  parent, then the clone root. Nothing to configure, and upstream having none is not an
  error, so check that the license you owe is there.
- The vendor branch holds only what sync writes. Everything else about the source — its
  patches, its hook registrations — lives on the branch you work on.

A repo `skillset` accepts is two things:

```text
sources.json   required, and `{}` is valid: a repo of nothing but your own skills
skills/        required, and may be empty
profiles.json  optional, named groups of skills to enable together
hooks/         optional, holds hooks.json and the scripts it names
licenses/      written by sync
```

`init` checks those first two and nothing else, so a new repo is:

```sh
mkdir -p <repo>/skills && cd <repo>
echo '{}' > sources.json
git init && git add -A && git commit -m "init"
skillset init <repo>
```

## Add a skill of your own

A skill you write goes straight into `skills/<name>/` on the branch, with no
`sources.json` entry. Sync never touches it, and the selection screen lists it under
`unlisted`, the group that holds every skill in the repo no source claims.

`isSkill()` is the whole test for what counts as a skill: `skills/<name>/SKILL.md` has to
exist. Nothing else is checked. The folder name is the skill's name everywhere — in
`skillset enable`, in the `deps` map, in the link in the agent folder, in the
`../<name>/SKILL.md` paths other skills use. All 49 skills here are lowercase with
hyphens, and all but `taste-skill` repeat their folder name in the frontmatter `name`.
Match that. The folder is linked whole, so references, scripts and subagent files sit
beside `SKILL.md` and reach the agent with it, the way `skills/tdd/mocking.md`,
`skills/diagnosing-bugs/scripts/` and `skills/research/agents/` do.

1. Write `SKILL.md`. Every skill here sets `name` and `description`; 18 also set
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

## Profiles

`profiles.json` is an optional object of profile names to a description and a flat skill
list. Profiles can mix skills from any source, and sync never changes the file.

```json
{
  "writing": {
    "description": "Draft and edit clear prose",
    "skills": ["unslop", "technical-writing"]
  }
}
```

`skillset enable writing` merges the members and their requirements into the current
selection; enabling another profile does not disable the first. `skillset disable writing`
turns off its listed members, even ones that were already on before it, except members
another enabled profile lists or a skill that stays on requires. Unlisted requirements stay
enabled, matching ordinary skill disable behavior. A same-named skill wins over a profile;
`profile:writing` selects the profile explicitly.

The machine config records the enabled profiles next to the flattened `enabled` list that
`apply` consumes. A config from before profiles needs no migration.

The selection screen shows profiles in their own section. Space merges or removes a profile,
and the middle column carries the same member skills with navigation back and forth. `n`
opens a blank editor for a new profile, and `e` edits the profile under the cursor; both work
on the skills step, with name, description and skills fields; tab and enter move between them, enter on
skills saves, and the CLI's name and member rules apply. Space on the skills field opens a
checklist of every skill, members checked: space toggles, `/` filters, enter keeps the
choice and esc drops it. Changing the name renames the
profile, including this machine's record of it. `d`
asks, and a second `d` deletes the profile under the cursor: it turns the profile and its
skills off (the skills stay in the repo; only the profile goes) and
joins the pending review, so save removes it from `profiles.json` and discarding keeps it. A
profile edit writes `profiles.json` at once; selection changes join the same pending review
as skill and target changes, and nothing links until save.

Editing is also on the command line, where agents and scripts change definitions:

```sh
skillset profile ls
skillset profile show <name>
skillset profile add <name> <skill>... [--desc <text>]
skillset profile set <name> [<skill>...] [--desc <text>]
skillset profile rm <name> [<skill>...]
```

`ls` marks the profiles on here with `*` and names members gone from the repo; `show` prints
one profile with each member's state. `rm <name> <skill>...` drops just those members, even
ones gone from the repo, and `skillset profile help` lists all of this. Unknown skill names
are all reported at once, each with a near match when there is one. When a skill and a
profile share a name, `enable` and `disable` act on the skill and print the `profile:` hint.

`add` creates the profile or merges the members in; `set` replaces the member list, or with
only `--desc` keeps it. A member name that isn't a skill in the repo is refused before
anything is written, as is a profile left with no skills. `--desc` takes the rest of the line
so a description with spaces needs no quoting; `--desc=<text>` works too and `--desc ""`
clears it. A new profile's name can't match a skill, start with `-` or `profile:`, or contain
spaces, so `skillset enable <name>` always reaches it. A repo with no `profiles.json` works as
before, and the first `profile add` creates the file. `rm` first turns the profile off on
this machine with the same rules as `disable`, so no skill is left that no profile can turn
off. Changing the members of a profile that is enabled on this machine keeps the selection
in step the same way: new members come on with their requirements, and dropped ones turn off
unless another enabled profile lists them or a skill that stays on requires them. The command then
says so and asks for `skillset apply`. Editing a profile that is off here changes only
`profiles.json`.

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

## Subagents

Claude Code loads subagents only from `~/.claude/agents`, so a subagent file inside a
skill folder is read by that skill, never registered. A skill that needs real subagents,
the way `cavecrew` sends work to `cavecrew-builder`, lists them in its `deps` entry:

```json
"cavecrew": { "agents": ["cavecrew-builder", "cavecrew-investigator", "cavecrew-reviewer"] }
```

Each name is `agents/<name>.md` in the repo, vendored through the source's `files`
globs (`"agents/cavecrew-*.md"`) or committed by hand for a skill of your own.
`skillset apply` links each one into `~/.claude/agents` while the skill is enabled and
removes the link when it isn't. The `agents` target has no subagent folder and gets none.
A name with no file prints `skip agent <name>.md: not in the repo` and links the rest.

## Finish

```sh
skillset sync <source>      # if a source changed
skillset enable <skill>     # on this machine
skillset apply
git push -u origin <branch>
```

Everything in `~/.config/skillset/` is this machine's own selection. It is outside the
repo; never add it to a commit.
