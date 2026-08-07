# Triage Labels

The skills speak in terms of canonical category and state roles. This file maps
those roles to the actual label strings used in this repo's issue tracker.

## Category roles

| Role in idp-xyz/idp-skills | Label in our tracker | Meaning                    |
| ------------------------- | -------------------- | -------------------------- |
| `bug`                     | `bug`                | Existing behaviour is broken |
| `enhancement`             | `enhancement`        | New capability or improvement |

## State roles

| Role in idp-xyz/idp-skills | Label in our tracker | Meaning                                      |
| ------------------------- | -------------------- | -------------------------------------------- |
| `needs-triage`            | `needs-triage`       | Maintainer needs to evaluate this issue      |
| `needs-info`              | `needs-info`         | Waiting on reporter for more information     |
| `ready-for-agent`         | `ready-for-agent`    | Fully specified and available for an agent   |
| `in-progress`             | `in-progress`        | Claimed work or a parent tracking active work |
| `ready-for-human`         | `ready-for-human`    | Requires human implementation                |
| `wontfix`                 | `wontfix`            | Will not be actioned                         |

When a skill mentions a role, use the corresponding label string from these
tables. An open **triage or implementation** item should have exactly one
category role and one state role. A closed remote issue is resolved and needs no
separate `resolved` label. Local markdown uses `Status: draft` only during
two-phase implementation-ticket publication and `Status: resolved` as its
terminal state; neither is an open triage role.

Wayfinder artifacts use a separate lifecycle and are excluded from triage
discovery. Hosted maps/children carry fixed `wayfinder:*` artifact labels and use
assignee plus open/closed state; local decision tickets use
`open` → `claimed` → `resolved` as defined in `issue-tracker.md`. They do not
receive the category/state roles above merely to satisfy triage.

Each right-hand role value must be non-empty and unique, case-insensitively.
Never map two roles to one label.

## Fixed Wayfinder labels

Hosted trackers also use these literal artifact-type labels:

- `wayfinder:map`
- `wayfinder:research`
- `wayfinder:prototype`
- `wayfinder:grilling`
- `wayfinder:task`

They are not state roles and are not remapped. Local markdown records the same
types in `Type:` instead.

Edit the right-hand role column to match whatever vocabulary you actually use.
