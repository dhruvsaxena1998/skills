# Triage Labels

The skills use six canonical workflow roles. This file maps those roles to the actual label strings used in this repo's issue tracker

| Label in skills   | Label in our tracker | Meaning                                                                          |
| ----------------- | -------------------- | -------------------------------------------------------------------------------- |
| `needs-info`      | `needs-info`         | waiting for reporter clarification. new reporter activity returns it to `triage` |
| `triage`          | `triage`             | needs evaluation, categorization, or clarification                               |
| `backlog`         | `backlog`            | captured but not yet reviewed or prioritized                                     |
| `ready-for-agent` | `ready-foragent`     | fully specified and safe for autonomous AFK implementation                       |
| `ready-for-human` | `ready-for-human`    | ready for a human to implement. Do not use this for “needs human input.”         |
| `in-progress`     | `in-progress`        | implementation has started                                                       |
| `blocked`         | `blocked`            | cannot proceed due to a dependency or unresolved decision                        |
| `in-review`       | `in-review`          | awaiting code review, validation, or acceptance                                  |
| `done`            | `done`               | accepted work completed.                                                         |
| `closed`          | `closed`             | resolved without delivering the requested work, such as invalid or duplicate     |
| `wontfix`         | `wontfix`            | consciously declined.                                                            |

When a skill mentions a workflow role, apply the corresponding tracker label from this table. Apply at most one workflow label to an issue at a time.

Edit the right-hand column to match the vocabulary already used in your tracker. Do not create duplicate labels when an equivalent existing label is available.

## Optional Classification Labels

Classification labels are independent of workflow status and may be applied alongside one workflow label.

| Classification | Available labels                                         | Meaning                               |
| -------------- | -------------------------------------------------------- | ------------------------------------- |
| Type           | `bug`, `feature`, `improvement`, `tech-debt`, `docs`     | Describes the kind of work or report  |
| Priority       | `critical`, `high`, `medium`, `low`                      | Indicates relative urgency and impact |
| Signal         | `security`, `breaking-change`, `duplicate`, `regression` | Indicates signals for triages         |

e.g. Use signal `duplicate` with status `closed`; it records why the issue was closed without adding another workflow state.
