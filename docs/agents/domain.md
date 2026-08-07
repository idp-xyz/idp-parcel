# Domain Docs

How the engineering skills should consume this repo's Bounded Context records, Ubiquitous Language, and architectural decisions when exploring the codebase.

## Before exploring, read these

- **`docs/domain/CONTEXT-MAP.md`** — the product context map. It points at one
  `CONTEXT.md` per established Bounded Context. Resolve context directories from
  these paths; do not assume contexts live under `src/` or at the repo root.
- **Per-context `CONTEXT.md`** — language, rules, lifecycles, and ownership for
  that context only (paths below).
- **`docs/domain/GLOSSARY.md`** — cross-context terms that must stay consistent.
- **`docs/domain/SCENARIOS.md`** — end-to-end scenarios that stress object
  identity and boundaries.
- **`docs/adr/`** — product-wide ADRs. All contexts currently share this
  directory; there is no per-context `docs/adr/` beside each `CONTEXT.md` yet.
  Number new ADRs by scanning `docs/adr/` only.

If a mapped `CONTEXT.md` is intentionally absent (currently
`collection-remittance` — first-release COD out of scope), record map-only
boundaries and do not invent empty context docs. If other domain files are
missing for an area under work, **proceed silently** and note the gap; lazy
creation belongs to `/domain-modeling` after language is resolved.

## File structure

```
/
├── AGENTS.md
├── docs/
│   ├── agents/                        ← this skill config
│   ├── domain/
│   │   ├── CONTEXT-MAP.md
│   │   ├── GLOSSARY.md
│   │   ├── SCENARIOS.md
│   │   ├── party-commercial/CONTEXT.md
│   │   ├── parcel-pricing/CONTEXT.md
│   │   ├── parcel-shipment/CONTEXT.md
│   │   ├── network-routing/CONTEXT.md
│   │   ├── node-operations/CONTEXT.md
│   │   ├── transport-fulfillment/CONTEXT.md
│   │   ├── customs-compliance/CONTEXT.md
│   │   ├── visibility-exception/CONTEXT.md
│   │   └── settlement-accounting/CONTEXT.md
│   └── adr/                           ← shared product ADRs
└── internal/<context>/                ← implementation packages (not domain authority)
```

Mapped context records (relative to `docs/domain/`):

| Context | CONTEXT.md |
| --- | --- |
| `party-commercial` | `party-commercial/CONTEXT.md` |
| `parcel-pricing` | `parcel-pricing/CONTEXT.md` |
| `parcel-shipment` | `parcel-shipment/CONTEXT.md` |
| `network-routing` | `network-routing/CONTEXT.md` |
| `node-operations` | `node-operations/CONTEXT.md` |
| `transport-fulfillment` | `transport-fulfillment/CONTEXT.md` |
| `customs-compliance` | `customs-compliance/CONTEXT.md` |
| `visibility-exception` | `visibility-exception/CONTEXT.md` |
| `settlement-accounting` | `settlement-accounting/CONTEXT.md` |
| `collection-remittance` | map-only until COD enters product scope |

For a future context-local ADR directory, write under `docs/adr/` relative to that
context's mapped `CONTEXT.md` directory and number by scanning only that target.
When one scope references an ADR in another, include the repository path as well
as the ADR number.

## Use each context's Ubiquitous Language

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), first identify the relevant Bounded Context, then use the term as defined in its `CONTEXT.md`. Don't drift to synonyms that record explicitly avoids in that context.

The same word may correctly carry a different meaning in another context. Translate at the boundary instead of forcing a global definition.

If the concept you need isn't recorded yet, that's a signal — either you're inventing language the model doesn't use (reconsider) or there is a real gap (note it for `/domain-modeling` or ask the user to invoke `/ubiquitous-language`).

Product and pilot authority stay outside this map: prefer
`docs/product/`, `docs/application/`, and `docs/design/` for release scope,
use cases, and handoffs; those documents must not rewrite domain ownership.

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (separate operational settlement from statutory finance) — but worth reopening because…_
