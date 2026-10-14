# Skills

The agent skills I use, collected from several upstream repos, with room for my own edits.
`skillset` links the ones I pick into my agent folders, per machine.

## Layout

```text
skills/<name>/SKILL.md   every skill, flat, whatever its source
sources.json             which skills to take from which upstream, and how they depend on each other
hooks/                   vendored hook scripts, plus hooks.json (mine) registering them
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
targets, enabled skills, and the skills it has already seen. That file is outside the repo
and is never committed, so switching a skill on or off creates no commit.

The first `init` on a machine adopts the skills already linked there as its selection, so
switching over removes nothing.

Targets are the agent folders to link into, both on by default:

| Target   | Skills            | Hooks                   | Read by                   |
| -------- | ----------------- | ----------------------- | ------------------------- |
| `claude` | `~/.claude/skills` | `~/.claude/settings.json` | Claude Code             |
| `agents` | `~/.agents/skills` | `~/.codex/hooks.json`     | Codex, pi, OpenCode     |

## The selection screen

Running `skillset` with no arguments opens it. The left column lists the sources, the
targets, and anything in an agent folder that this repo didn't put there; the middle column
lists the selected source's skills; the right pane shows the skill under the cursor — its
description, source, dependencies, dependents and local patch commits. New skills since the
last run are marked, and stay off until you turn them on.

Toggling a skill applies the dependency rules and says what it dragged along: enabling pulls
in what the skill requires, disabling unchecks the skills that require it, and a skill left
without one of its optional dependencies keeps a warning on screen.

`space` toggles, `/` filters, `enter` on a skill reads its SKILL.md, `s` reviews every link, unlink and hook change the selection
would write, `enter` on that review saves and applies it, `p` pulls the repo, `q` quits.
`?` opens the full key list and the settings. Nothing is written until the review is saved,
and the screen's only git action is `git pull --ff-only`. It never commits or pushes.

## Commands

```text
skillset                 open the selection screen
skillset init [path]     remember the skills clone (clones one if there is none)
skillset enable <skill>  turn a skill on here, with what it requires
skillset disable <skill> turn it off, with the skills that require it
skillset apply           link enabled skills, unlink the rest
skillset sync <source>   pull a source's skills from upstream into this repo
skillset update          replace this binary with the latest release
```

`enable` and `disable` change the selection without the screen, which is how agents and
scripts change it. Neither one links anything; `apply` does that.

`apply` writes what the review shows: it links every enabled skill into every enabled
target, removes the links that point into this repo for everything else, and leaves entries
that point elsewhere alone. Links are symlinks, so edits in this repo take effect
immediately. It also makes each target's hook config match the selection, registering the
entries in `hooks/hooks.json` for enabled skills only, after any other tool's entries, and
backing the file up to `.bak` before writing. Codex asks you to trust changed hooks on its
next start. Run `apply` after a pull.

## Sources

| Source       | Upstream                                                                      | Branch              |
| ------------ | ----------------------------------------------------------------------------- | ------------------- |
| `mattpocock` | [mattpocock/skills](https://github.com/mattpocock/skills)                     | `vendor/mattpocock` |
| `pstack`     | [cursor/plugins › pstack](https://github.com/cursor/plugins/tree/main/pstack) | `vendor/pstack`     |
| `ponytail`   | [dietrichgebert/ponytail](https://github.com/dietrichgebert/ponytail)         | `vendor/ponytail`   |

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
| `blast-radius`                                                                                             | Removed `arena`/`how`/`why` steps; pulls PRs with `gh` or MRs with `glab`           | Those pstack skills only work in Cursor; support GitHub and GitLab |
| `to-spec`, `to-tickets`, `to-questionnaire`, `research`, `domain-modeling`, `handoff`, `technical-writing` | Write their output through `unslop`                                                 | Consistent, plain prose in every artifact                          |
| `typescript-best-practices`                                                                                | Dropped `disable-model-invocation`; principles linked by path                       | Applies automatically when editing `.ts`                           |
| `tdd`                                                                                                      | Added "When not to add a test" from pstack's `tdd`                                  | Prefer no test over a bad test                                     |
| `to-spec`, `to-tickets`, `implement-spec` | Without `/setup-matt-pocock-skills`, default to untracked local markdown in `.scratch/` (excluded via `.git/info/exclude`); GitHub, GitLab or tracked markdown only when asked | They work in any repo without a setup step or touching tracked files |

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
