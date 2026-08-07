# Issue tracker: Local Markdown

Issues and specs (you may know a spec as a PRD) for this repo live as markdown files in `.scratch/`.

## Conventions

- One feature per directory: `.scratch/<feature-slug>/`
- The spec is `.scratch/<feature-slug>/spec.md`
- Implementation issues are one file per ticket at `.scratch/<feature-slug>/issues/<NN>-<slug>.md`, numbered from `01` — never a single combined tickets file
- Category and state are recorded as `Category:` and `Status:` lines near the top
  of each issue/spec file (see `triage-labels.md` for the role strings)
- Comments and conversation history append to the bottom of the file under a `## Comments` heading

## When a skill says "publish to the issue tracker"

Create a new file under `.scratch/<feature-slug>/` (creating the directory if needed).

## When a skill says "fetch the relevant ticket"

Read the file at the referenced path. The user will normally pass the path or the issue number directly.

## Implementation ticket operations

Used by `/to-tickets` and `/implement`.

- **Draft and activate children**: create every child with `Status: draft` and
  write all blocking edges. When a parent spec exists, record the complete
  numbered child list there and set it to `Status: in-progress` before changing
  children to `Status: ready-for-agent`. Without a parent, activate the fully
  wired children directly.
- **Claim**: change the chosen child from `Status: ready-for-agent` to
  `Status: in-progress` before coding.
- **Complete**: change it to `Status: resolved` and append the commit SHA and
  verification result.
- **Complete a parent**: read every file in the recorded child set. Set the parent
  spec to `Status: resolved` only when every child says `Status: resolved`;
  otherwise leave it `in-progress`.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a file with one **child** file per ticket.

- **Map**: `.scratch/<effort>/map.md` — the Notes / Decisions-so-far / Fog body.
- **Child ticket**: `.scratch/<effort>/issues/NN-<slug>.md`, numbered from `01`,
  with the question in the body. A `Type:` line records the ticket type
  (`research`/`prototype`/`grilling`/`task`); a `Status:` line records exactly
  one of `open`, `claimed`, or `resolved`. Create every new child as
  `Status: open`.
- **Blocking**: a `Blocked by: NN, NN` line near the top. A ticket is unblocked when every file it lists is `resolved`.
- **Frontier**: scan `.scratch/<effort>/issues/` for files with `Status: open`
  whose blockers are all resolved; first by number wins.
- **Claim**: change `Status: open` to `Status: claimed` and save before any work.
- **Resolve**: append the answer under an `## Answer` heading, set `Status: resolved`, then append a context pointer (gist + link) to the map's Decisions-so-far in `map.md`.
- **Handoff**: when the map is complete, write
  `.scratch/<effort>/handoff.md` and point the map's `## Handoff` section at it.
  Consumers must read that file; Decisions-so-far gists are not the full decision
  record.
