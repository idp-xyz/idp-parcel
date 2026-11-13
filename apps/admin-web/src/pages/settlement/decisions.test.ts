import { deepEqual, equal, match, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { chargeDecisionTargets, settlementDecisionEndpoints, settlementDecisionHints, statementDecisionTargets } from './decisions';

test('账务的每个运营决定口都有路径与填写提示', () => {
  for (const kind of Object.keys(settlementDecisionEndpoints) as (keyof typeof settlementDecisionEndpoints)[]) {
    match(settlementDecisionEndpoints[kind], /^\/settlement-[a-z-]+$/);
    ok(settlementDecisionHints[kind].includes('请填'), kind);
  }
});

test('费用页挂确认与调整，对账页挂发布、作废与两种纳入', () => {
  deepEqual(
    chargeDecisionTargets.map((target) => target.endpoint),
    ['POST /settlement-charge-confirmations', 'POST /settlement-charge-adjustments'],
  );
  deepEqual(
    statementDecisionTargets.map((target) => target.endpoint),
    [
      'POST /settlement-statement-publications',
      'POST /settlement-statement-voids',
      'POST /settlement-late-charge-inclusions',
      'POST /settlement-adjustment-inclusions',
    ],
  );
  equal(new Set([...chargeDecisionTargets, ...statementDecisionTargets].map((target) => target.id)).size, 6);
});
