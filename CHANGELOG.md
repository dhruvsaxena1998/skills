# Changelog

Changes to `skillset` and to the skills this repo ships. Each release's commit list is on
its [GitHub release](https://github.com/dhruvsaxena1998/skills/releases).

## v0.1.6 — 2026-10-28

### Added

- A skill's `deps` entry can list `agents`. `skillset apply` links each `agents/<name>.md`
  into `~/.claude/agents` while the skill is enabled, and removes the link when it isn't.
- The `caveman` source: 16 skills, its hooks, and the `cavecrew` subagents (#36).

### Fixed

- `skillset apply` treats a hook as its own when the command runs any script in the repo,
  not only one in `hooks/`. Hooks vendored at another path, like caveman's `src/hooks/`,
  were added again on every `apply` and never removed.

## v0.1.5 — 2026-10-28

### Added

- `skillset playbook` prints `AGENTS.md`, so a new skills repo gets its setup instructions
  from the binary instead of a copy of the file.
- The `taste` source, with `taste-skill`.

## v0.1.4 — 2026-10-18

### Changed

- The skill preview renders Markdown with a small built-in renderer instead of
  Glamour. It draws headings, paragraphs, lists, block quotes, fenced code, and
  inline code, emphasis and links. The stripped binary is roughly half the size:
  6.80 MiB, down from 13.55 MiB (#31).

## v0.1.3 — 2026-10-17

### Added

- The `skillset` screen syncs sources: `u` syncs the source under the cursor, `U` syncs
  every source.
- pstack's `arena`, `how` and `why` skills. Their seats run on Claude and Codex instead of
  Cursor, and `blast-radius` links to them again.

### Fixed

- Picking a theme no longer writes skill and target toggles you haven't saved. Before,
  quitting with `q` `q` after a theme pick left them in the config, and the next `apply`
  linked them (#25).
- `skillset init <new path>` removes the hook entries the old clone registered in
  `~/.claude/settings.json` and `~/.codex/hooks.json`. Before, those entries stayed and
  failed on every event once the old clone was deleted (#24).
- The screen starts no git job while another is running. Before, `U` then `p`, or `u`
  twice, ran two jobs on the same repo and one failed on `.git/index.lock` (#23).
- A path typed at the `skillset init` clone prompt that starts with `~/` now expands to
  the home folder, instead of cloning into a folder named `~` (#22).
- `sources.json` keeps its own layout when sync rewrites it.

## v0.1.2 — 2026-10-14

### Added

- `skillset version`, and `skillset update` skips the download when the binary is current.
- The review screen reads as a diff, with a wider first column and a visible cursor.

## v0.1.1 — 2026-10-14

### Added

- Settings you move through with the keys.
- The screen never blocks while drawing, works with the mouse, and draws square boxes.

### Fixed

- Downloads in the installers say what they are doing instead of sitting silent.
- The overlay cursor and mouse clicks in the key list.

## v0.1.0 — 2026-10-14

First release of `skillset`, the Go binary that replaced the bash scripts: `init`,
`apply`, `enable` and `disable` with dependency handling, `sync` from upstream, hooks
that follow their skill, the selection screen, installers for macOS and Windows, and
`skillset update`.
