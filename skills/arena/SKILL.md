---
name: arena
description: "Spawn N parallel candidates at the same task, pick a base, graft the strongest parts of the losers into it. Use for /arena, 'arena this', 'throw it in the arena', or when one attempt at a non-trivial artifact would lock in the wrong shape."
disable-model-invocation: true
---

# Arena

Open a todolist with one entry per phase (Frame, Fan out, Cross-judge, Pick, Graft, Verify) before launching anything.

## Phase A: Frame

Every candidate gets the same prompt, so the prompt is the contract.

1. State the artifact each candidate is producing.
2. Derive the rubric, 3-6 concrete gradeable criteria for what success looks like on *this* task. It is the picker's tool in Phase D. Candidates only see the task.
3. Pick the runners. A seat is `claude:<opus|sonnet|haiku>` or `codex:<sol|terra|luna>`. A `claude:` seat is an Agent tool subagent with `model` set to that alias. A `codex:` seat is a Bash call with `run_in_background: true`: `codex exec -m <slug> -c model_reasoning_effort=<low|medium|high|xhigh> -s <sandbox> -C <dir> -o <file> "<prompt>"`, where `sol` is `gpt-6.1-sol`, `terra` is `gpt-5.6-terra` and `luna` is `gpt-6-luna`, and `-o` names the file its final answer lands in. Seats the user names when invoking the skill replace the defaults below. If `codex` isn't installed or rejects a slug, run that seat as `claude:opus` and say so. Pick each seat's model and reasoning effort for the job: higher for judgment, lower for search. Default to one `claude:opus` and one `codex:sol`. Spawn more runners when the arena covers multiple design directions. Use the same model N times when the work is generation-bound rather than judgment-sensitive.
4. Give each candidate its own output location. A `claude:` seat gets `isolation: "worktree"`. A `codex:` seat gets `git worktree add /tmp/arena-<slug>/candidate-<n>` and runs there with `-s workspace-write`, with `-o /tmp/arena-<slug>/candidate-<n>.md`. Outside a git repo, use the bare `/tmp/arena-<slug>/candidate-<n>/` folder and add `--skip-git-repo-check`.

## Phase B: Fan out

Launch all N seats in one message, each with the task, the path to the shared grounding, its own output path, and instructions to produce the artifact and a short rationale naming the alternatives it considered and what it rejected.

If a candidate produces no output, proceed with N-1 and note the dropout.

## Phase C: Cross-judge

After all Phase B candidates complete, choose the judge from `claude:opus` and `codex:sol`, or a seat the user named. Prefer a different model family from the parent's, which in Claude Code means `codex:sol`. Spawn one readonly judge on that seat: `-s read-only` for codex, and an instruction not to edit anything for claude. It sees the rubric and the candidates by path label, scores each criterion, and recommends a base with rationale. It runs in parallel with your reading in Phase D. Don't spawn it while candidates are still writing.

## Phase D: Pick a base

Read every candidate end to end before picking. Score each against the rubric criterion by criterion, not on holistic feel, and compare with the cross-judge. Agreement on the base confirms the pick. Disagreement means one of you is biased or the rubric was ambiguous. Read both rationales before deciding.

Pick the base a future maintainer can extend most easily without breaking invariants. When two feel tied, prefer the cleaner boundary or smaller API.

## Phase E: Graft

Walk each losing candidate once more for what is worth porting into the base, usually one or two things per candidate, not most of it. Fold each graft in by hand so the result stays coherent under one mental model. Don't paste mechanically.

If the candidates converge on one shape, that is strong agreement. Note it and ship the consensus shape. No graft is needed. If they wildly diverge, Phase A was under-specified. Reframe and re-run rather than averaging the divergence.

## Phase F: Verify

Verify the synthesized artifact like any other output: run it, test it, and show the evidence. If verification finds a problem the arena missed, either Phase A was wrong (re-frame and re-run) or a candidate caught it and you missed the graft (go back to Phase E). Don't paper over it.

## Outputs

One synthesized artifact, with a short synthesis note alongside naming the base and why, the cross-judge's verdict, each graft with its source candidate, what was rejected and why, any convergence or dropouts, and the verification result.
