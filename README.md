# Skills

A collection of reusable agent skills and a Bun utility for installing them as symbolic links for supported coding agents.

## Included Skills

| Skill                                          | Purpose                                                                                       |
| ---------------------------------------------- | --------------------------------------------------------------------------------------------- |
| [`domain-modeling`](skills/domain-modeling/SKILL.md) | Build and sharpen a project's domain model, glossary, and architecture decisions.       |
| [`grilling`](skills/grilling/SKILL.md)         | Stress-test a plan, decision, or idea through structured rounds of questions.                 |
| [`setup-skills`](skills/setup-skills/SKILL.md) | Configure a repository's issue tracking, triage labels, and domain-documentation conventions. |
| [`triage`](skills/triage/SKILL.md)             | Reserved skill directory; its `SKILL.md` is currently empty.                                  |

Each skill lives in `skills/<name>/` and is recognized when the directory contains a `SKILL.md` file. A skill may also include agent-specific metadata under `agents/`.

## Requirements

- [Bun](https://bun.sh/)

## Install

Clone the repository and install dependencies:

```sh
bun install
```

Link all skills into the global directories for Agents, Claude, and Pi:

```sh
bun run link
```

This creates symbolic links in:

```text
~/.agents/skills
~/.claude/skills
~/.pi/agent/skills
```

## Manage Links

Preview changes without modifying the filesystem:

```sh
bun run link --dry-run
```

Link skills for specific agents only:

```sh
bun run link --agents=claude,pi
```

Link skills for every supported agent except one:

```sh
bun run link --skip-agents=pi
```

Install skills into a project's local agent directories instead of the global directories:

```sh
bun run link --project /path/to/project
```

Check the current link state:

```sh
bun run status
```

Remove only links that point back to this repository:

```sh
bun run unlink
```

Existing files and links owned by another source are left unchanged. Pass `--force` to `link` only when those foreign symbolic links should be replaced.

## Add a Skill

1. Create `skills/<skill-name>/SKILL.md`.
2. Add YAML front matter with the skill name and description.
3. Optionally add `skills/<skill-name>/agents/openai.yaml` for display metadata.
4. Run `bun run link` to install the new skill.

The linker automatically discovers all immediate subdirectories of `skills/` that contain a `SKILL.md` file.

## Development

Type-check the linking utility:

```sh
bunx tsc --noEmit
```
