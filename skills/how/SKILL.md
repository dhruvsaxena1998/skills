---
name: how
description: "Use for \"how does X work\", code walkthroughs before changing something, and placement / ownership / layering questions (\"where should this live\", \"which package owns this\", \"is this the right layer\"). Explains subsystem architecture, runtime flow, onboarding mental models. Use why for motivation."
disable-model-invocation: true
---

# How

Explore the codebase to answer "how does X work?" questions. Produce architectural explanations at the level of a senior engineer onboarding onto a subsystem, enough to build a working mental model, not so much that it reads like annotated source code.

A seat is `claude:<opus|sonnet|haiku>` or `codex:<sol|terra|luna>`. A `claude:` seat is an Agent tool subagent with `model` set to that alias. A `codex:` seat is a Bash call with `run_in_background: true`: `codex exec -m <slug> -c model_reasoning_effort=<low|medium|high|xhigh> -s <sandbox> -C <dir> -o <file> "<prompt>"`, where `sol` is `gpt-6.1-sol`, `terra` is `gpt-5.6-terra` and `luna` is `gpt-6-luna`, and `-o` names the file its final answer lands in. Seats the user names when invoking the skill replace the defaults below. If `codex` isn't installed or rejects a slug, run that seat as `claude:opus` and say so. Pick each seat's model and reasoning effort for the job: higher for judgment, lower for search. Explorers default to `codex:terra`, and every explainer to `claude:opus`. Every spawn is read-only: `-s read-only -C <repo root>` for codex, and an instruction not to edit anything for claude.

## 1. Assess complexity

If the scope is ambiguous, state your interpretation and explore. The user can redirect.

- **Simple** (a single module, a small utility, a narrow question such as "how does function X work"): no explorers. Spawn one explainer that explores and explains in one pass, built from `references/explainer-prompt.md` without the explorer-findings section. Go to step 4.
- **Complex** (a subsystem spanning multiple files or services, a cross-cutting feature, a full architectural overview): go to step 2.

When in doubt, take the simple path.

## 2. Explore (complex only)

Decompose the question into 2 to 4 angles, each a distinct slice of the subsystem. Spawn all explorers in a single message, each with `references/explorer-prompt.md` and its angle filled in.

## 3. Synthesize (complex only)

Once all explorers have returned, spawn one explainer with `references/explainer-prompt.md` and every explorer's findings filled in.

## 4. Present

Present the explainer's output. Light edits for clarity or context from the conversation are fine. Do not substantially rewrite it.

## Output format

The explanation uses the sections defined in `references/explainer-prompt.md`, dropping any that do not apply: Overview, Key Concepts, How It Works, Where Things Live, Gotchas.
