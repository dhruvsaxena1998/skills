# Domain docs

This repo uses a single context. Domain terms belong in the root `GLOSSARY.md`; architecture decisions belong in `docs/adr/`.

## Before exploring

Read the glossary and any ADRs relevant to the work. If they do not exist, proceed without creating placeholders. The domain-modeling skill creates them when terms or decisions are resolved.

## Use the glossary vocabulary

Use the glossary's terms in specs, issues, code, and tests. If a needed concept has no definition, check the codebase before introducing a new term. Record a definition through domain-modeling when the distinction matters.

## Flag ADR conflicts

When a proposal contradicts an existing ADR, identify that ADR and explain why the decision should be reconsidered. Do not silently override it.
