# Skills

The agent skills I use, collected from several upstream repos, with room for my own edits.
`skillset` links the ones I pick into my agent folders, per machine.

## Layout

```text
skills/<name>/SKILL.md   every skill, flat, whatever its source
sources.json             which skills to take from which upstream, and how they depend on each other
profiles.json            optional named groups of skills to enable together
hooks/                   vendored hook scripts, plus hooks.json (mine) registering them
agents/                  vendored subagents, linked while the skill that ships them is enabled
src/hooks/               caveman's hook scripts, vendored where its scripts expect them
licenses/                upstream licenses
cmd/skillset/            the skillset binary
AGENTS.md                how to add a source, a skill, a patch or a hook
```

## Install

macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/dhruvsaxena1998/skills/main/install.sh | bash
```

Windows:

```powershell
irm https://raw.githubusercontent.com/dhruvsaxena1998/skills/main/install.ps1 | iex
```

Both download the latest release build for the machine's architecture: `~/.local/bin` on
macOS, `%USERPROFILE%\.local\bin` on Windows, where the installer also puts that folder on
the user PATH. Nothing else is needed to run `skillset`; `git` is needed for `sync` and for
the pull key in the selection screen, and `node` for ponytail's hooks.

`skillset update` replaces the binary with the latest release.

## Set up a machine

```sh
skillset init            # inside a clone, or anywhere to clone one
skillset init ~/skills   # or name the clone, e.g. after moving it
```

`init` walks up from the working directory looking for a clone and offers to clone one into
`~/skills` if it finds none. It saves the path in `~/.config/skillset/config.json`
(`%USERPROFILE%\.config\skillset\config.json` on Windows) along with this machine's enabled
targets, directly enabled skills, enabled profiles, and the skills it has already seen. That file is outside the repo
and is never committed, so switching a skill on or off creates no commit.

The first `init` on a machine adopts the skills already linked there as its selection, so
switching over removes nothing.

Targets are the agent folders to link into, both on by default:

| Target   | Skills             | Hooks                     | Subagents          | Read by             |
| -------- | ------------------ | ------------------------- | ------------------ | ------------------- |
| `claude` | `~/.claude/skills` | `~/.claude/settings.json` | `~/.claude/agents` | Claude Code         |
| `agents` | `~/.agents/skills` | `~/.codex/hooks.json`     | none               | Codex, pi, OpenCode |

## Your own skills repo

`skillset` is not tied to this repo. `init` takes any folder that has a `sources.json` and
a `skills/` folder; that is the whole test. The vendor branches, `hooks/hooks.json` and
`licenses/` are made when they are first needed.

Fork this one to start from these skills and these patches:

```sh
git clone https://github.com/<you>/skills ~/skills
cd ~/skills
git fetch origin 'refs/heads/vendor/*:refs/heads/vendor/*'   # once per clone, see below
skillset init ~/skills
skillset apply
```

Start empty to take only the machinery:

```sh
mkdir -p ~/skills/skills && cd ~/skills
echo '{}' > sources.json
git init && git add -A && git commit -m "init"
skillset init ~/skills
```

An empty `sources.json` is valid. With no source, every skill in `skills/` is `unlisted` on
the selection screen, which is where a skill of your own lives in any case.

From there [AGENTS.md](AGENTS.md) is the playbook and needs no changes: it is written about
a skills repo, not about this one. Adding a source, writing a skill of your own, patching an
upstream skill and registering a hook all work the same in a fork or an empty repo. Keep
`AGENTS.md` and its one-line `CLAUDE.md` (`@AGENTS.md`) so your agent reads it too.

To keep taking my changes after a fork:

```sh
git remote add upstream https://github.com/dhruvsaxena1998/skills
git pull upstream main
```

My patches arrive as ordinary commits and meet yours in a normal merge, the same way an
upstream sync does.

Two things point at this repo whatever repo you link from:

- `skillset init` with no path offers to clone *this* repo when it finds no clone. Pass your
  path and it never asks.
- `skillset update` installs *this* repo's releases. If you fork the Go code, build it with
  `go build ./cmd/skillset` and don't run `update`: it would replace your build with mine.

## The selection screen

Running `skillset` with no arguments opens it. The left column lists the sources, profiles,
targets, and anything in an agent folder that this repo didn't put there; the middle column
lists the selected source or profile's skills; the right pane shows the row under the cursor —
its description, source, dependencies, dependents and local patch commits. New skills since
the last run are marked, and stay off until you turn them on.

Toggling a skill applies the dependency rules and says what it dragged along: enabling pulls
in what the skill requires, disabling unchecks the skills that require it, and a skill left
without one of its optional dependencies keeps a warning on screen. A profile's checkbox
shows whether that profile is enabled; its tally shows how many member skills are currently
on, so individually toggled members never make the profile state ambiguous.

`space` toggles, `/` filters, `enter` on a skill reads its SKILL.md, `s` reviews every link, unlink and hook change the selection
would write, `enter` on that review saves and applies it, `p` pulls the repo, `u` runs
`skillset sync` for the source under the cursor and `U` for every source, `q` quits.
`?` opens the full key list and the settings. Nothing is written until the review is saved.
The screen's git actions are `git pull --ff-only` and sync's vendor commit and merge. It never pushes.

## Commands

```text
skillset                 open the selection screen
skillset init [path]     remember the skills clone (clones one if there is none)
skillset enable <skill|profile>  turn a skill on, or merge a profile in
skillset disable <skill|profile> turn a skill or profile off
skillset apply           link enabled skills, unlink the rest
skillset sync <source>   pull a source's skills from upstream into this repo
skillset profile ...     create, edit and delete profiles (ls, add, rm, set)
skillset playbook        print the playbook (AGENTS.md) for setting up a skills repo
skillset update          replace this binary with the latest release
skillset version         print the version of this binary (--version, -v)
```

`playbook` prints `AGENTS.md`, the recipes for adding sources, skills, patches and hooks.
To start a skills repo of your own, hand it to your agent instead of copying the file:
`skillset playbook > AGENTS.md` in the new repo, then `echo '@AGENTS.md' > CLAUDE.md`.

`enable` and `disable` change the selection without the screen, which is how agents and
scripts change it. Neither one links anything; `apply` does that.

### Profiles

A repo can define named skill groups in `profiles.json`. The file is optional; without it,
the commands and selection screen work as before.

```json
{
  "writing": {
    "description": "Draft and edit clear prose",
    "skills": ["unslop", "technical-writing"]
  },
  "review": {
    "skills": ["pr", "blast-radius", "unslop"]
  }
}
```

```sh
skillset enable writing
skillset enable review   # merges with writing; nothing is switched off
skillset disable writing # review and direct selections keep unslop on
skillset apply
```

Enabling a profile adds its members and their requirements and records the profile as
enabled. Enabling another profile is cumulative. Disabling a profile considers only its
listed members: a member stays on when it was enabled directly, belongs to another enabled
profile, or is required by a skill that remains enabled. Requirements that are not listed
members are left on, just as they are when an individual skill is disabled.

A skill with the same name as a profile takes precedence. Use `profile:<name>` to select the
profile explicitly, for example `skillset enable profile:writing`.

### Managing profiles

The `profile` command creates, edits and deletes profiles in `profiles.json`. Editing or
deleting a profile that is enabled on this machine brings the selection along (new members
on, dropped members off unless something else keeps them) and asks for `apply`; a profile
that is off here changes only in the file.

```sh
skillset profile ls                                  # list profiles and their members
skillset profile add writing technical-writing unslop # create writing, or add to it
skillset profile add writing --desc "clear prose"     # set or replace the description
skillset profile set review pr blast-radius unslop    # replace review's members
skillset profile rm writing                            # turn writing off, then delete it
```

`add` creates the profile if it doesn't exist and merges any new members into it. `set`
replaces the member list; given only `--desc`, it changes the description and keeps the
members. `--desc=<text>` works too, and `--desc ""` clears it. Skill names must match their
folders; an unknown name is refused before anything is written, and so is a profile left
with no skills. A new profile's name can't match a skill, start with `-` or `profile:`, or
contain spaces, so `skillset enable <name>` always reaches it. If a skill added later takes a
profile's name, the skill wins for `enable`/`disable` and `profile:<name>` forces the
profile. A repo without
`profiles.json` works as before; the first `profile add` creates the file.

In the selection screen, profiles have their own section. Move right to inspect or toggle
individual members, left to return to the profile, and space on the profile to merge or
remove the whole selection. `n` opens the profile editor (empty to create one, or the
profile under the cursor to edit it). `d` asks, and a second `d` deletes the profile under
the cursor; like `profile rm` it turns the profile off first, and the delete waits for save,
so discarding keeps it. Profile edits write `profiles.json` at once; the selection changes still go through the review-and-
save step.

### Apply

`apply` writes what the review shows: it links every enabled skill into every enabled
target, removes the links that point into this repo for everything else, and leaves entries
that point elsewhere alone. Links are symlinks, so edits in this repo take effect
immediately. It also makes each target's hook config match the selection, registering the
entries in `hooks/hooks.json` for enabled skills only, after any other tool's entries, and
backing the file up to `.bak` before writing. Codex asks you to trust changed hooks on its
next start. Run `apply` after a pull.

A skill whose `deps` entry lists `agents` gets each `agents/<name>.md` linked into
`~/.claude/agents` while it is enabled, and pruned the same way skill links are. The
`agents` target has no subagent folder, so it links none.

## Sources

| Source       | Upstream                                                                      | Branch              |
| ------------ | ----------------------------------------------------------------------------- | ------------------- |
| `mattpocock` | [mattpocock/skills](https://github.com/mattpocock/skills)                     | `vendor/mattpocock` |
| `pstack`     | [cursor/plugins › pstack](https://github.com/cursor/plugins/tree/main/pstack) | `vendor/pstack`     |
| `ponytail`   | [dietrichgebert/ponytail](https://github.com/dietrichgebert/ponytail)         | `vendor/ponytail`   |
| `taste`      | [Leonxlnx/taste-skill](https://github.com/Leonxlnx/taste-skill)                 | `vendor/taste`      |
| `caveman`    | [JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman)               | `vendor/caveman`    |
| `plannotator` | [backnotprop/plannotator › apps/skills/core](https://github.com/backnotprop/plannotator/tree/main/apps/skills/core) | `vendor/plannotator` |

## How upstream sync works

Each source has a `vendor/<source>` branch that holds only that source's skills, exactly as
upstream ships them. It is never edited by hand. `main` merges those branches and carries my
edits as ordinary commits on top.

```sh
skillset sync mattpocock
```

This clones upstream, replaces the selected skills on `vendor/mattpocock`, commits
`sync mattpocock @ <sha>`, and merges that branch into the current one. Git's 3-way merge
keeps my edits wherever upstream didn't touch the same lines; anything else shows up as a
normal merge conflict. Sync refuses to start with uncommitted changes, and warns when a
skill links to a sibling skill that its `deps` entry doesn't list.

`git clone` brings only `main`, so a fresh clone has no local `vendor/*` branches. Sync
rebuilds a missing one as a new orphan with no history in common with the merge, and every
file I have patched then conflicts add/add. Fetch them once per clone:

```sh
git fetch origin 'refs/heads/vendor/*:refs/heads/vendor/*'
```

See what I've changed relative to upstream:

```sh
git diff vendor/mattpocock main -- skills/tdd
```

Adding a source, a skill, a patch or a hook is in [AGENTS.md](AGENTS.md).

## Patches

My edits to upstream skills, each in its own `patch` commit. List them with
`git log --oneline --grep '^patch'`.

| Skill                                                                                                      | Change                                                                              | Why                                                                |
| ---------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `unslop`                                                                                                   | Dropped `disable-model-invocation`; the description says when to apply it           | Agents run it on their own when writing prose                      |
| `pr`                                                                                                       | Prose goes through `unslop`; **Blast Radius** is filled by following `blast-radius` | Combine Matt's PR template with pstack's writing and risk checks   |
| `blast-radius` | Links `arena`/`how`/`why` by path, with a subagent fallback when `arena` is off; pulls PRs with `gh` or MRs with `glab` | Those skills are reached by path here; support GitHub and GitLab |
| `to-spec`, `to-tickets`, `to-questionnaire`, `research`, `domain-modeling`, `handoff`, `technical-writing` | Write their output through `unslop`                                                 | Consistent, plain prose in every artifact                          |
| `typescript-best-practices`                                                                                | Dropped `disable-model-invocation`; principles linked by path                       | Applies automatically when editing `.ts`                           |
| `tdd`                                                                                                      | Added "When not to add a test" from pstack's `tdd`                                  | Prefer no test over a bad test                                     |
| `to-spec`, `to-tickets`, `implement-spec` | Without `/setup-matt-pocock-skills`, default to untracked local markdown in `.scratch/` (excluded via `.git/info/exclude`); GitHub, GitLab or tracked markdown only when asked | They work in any repo without a setup step or touching tracked files |
| `arena`, `how`, `why` | Subagents run as `claude:opus/sonnet/haiku` or `codex:sol/terra/luna` seats instead of Cursor models; MCPs found from the session's tools | Run in Claude Code, mixing Claude and Codex models |

Skills reference each other by relative path (`../unslop/SKILL.md`), not by name, because
most pstack skills set `disable-model-invocation` and can't be called through the Skill
tool.

## Windows

`skillset` is a single binary: no Git Bash, no `jq`, no Go. Install it with `install.ps1`
and open a new terminal so the PATH change is picked up.

Linking prefers a real symlink, which needs Developer Mode (Settings → System → For
developers). Without it, `apply` falls back to a directory junction, which needs no
privilege. Either way the agent folders are under `%USERPROFILE%`.

Hook commands are written with forward slashes so Windows `node` can open them. ponytail's
statusline uses `hooks/ponytail-statusline.ps1`. `.gitattributes` keeps every file LF except
`.ps1`, so upstream syncs merge cleanly.
