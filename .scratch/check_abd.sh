#!/usr/bin/env bash
set -euo pipefail
cd /home/tops/workspace/idp-parcel-landing
for f in docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md \
         docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md \
         docs/domain/CONTEXT-MAP.md docs/domain/parcel-pricing/CONTEXT.md; do
  echo "== $f"
  echo -n "  冲突标记: "; git show abd4c65bc:$f | grep -c '<<<<<<<\|>>>>>>>' || true
  echo -n "  ADR-0378 引用: "; git show abd4c65bc:$f | grep -c 'ADR-0378' || true
  echo -n "  main0380 引用: "; git show abd4c65bc:$f | grep -c '0380-the-customer-contract' || true
done
