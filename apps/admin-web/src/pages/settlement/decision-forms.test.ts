import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { buildGuidedPayload, carriedGuidedValues, initialGuidedValues } from '../../components/guided-form/guided-forms';
import { chargeDecisionPrefill, settlementDecisionForm, statementDecisionPrefill } from './decision-forms';

test('点中的费用带进确认与调整，点中的对账单不带进发布', () => {
  deepEqual(Object.keys(chargeDecisionPrefill('CHG')), ['charge-confirmation', 'charge-adjustment']);
  equal(statementDecisionPrefill('ST-1')['statement-publication'], undefined);
  deepEqual(statementDecisionPrefill('ST-1')['statement-void'], { statement: 'ST-1' });
  deepEqual(chargeDecisionPrefill(null), {});
});

test('带出的标识只写进表单认得的格', () => {
  const fields = settlementDecisionForm('statement-void').fields;
  deepEqual(carriedGuidedValues(fields, { statement: 'ST-1', charge: 'CHG' }), { statement: 'ST-1' });
  deepEqual(carriedGuidedValues(fields, undefined), {});
});

test('发布不列费用也不勾带走调整时交不出去，勾了可以不列', () => {
  const definition = settlementDecisionForm('statement-publication');
  const base = {
    ...initialGuidedValues(definition.fields),
    account: 'ACCT',
    period: '2026-10',
    version: 'v1',
    currency: 'CNY',
    lastAttributionDate: '2026-10-31',
    declaredTotal: '0',
    cutOffAt: '2026-10-31T16:00:00Z',
  };
  equal(buildGuidedPayload(definition, base).ok, false);
  const carried = buildGuidedPayload(definition, { ...base, carryUnbilledAdjustments: true });
  equal(carried.ok, true);
  if (carried.ok) equal(carried.payload.carryUnbilledAdjustments, true);
});

test('费用调整的金额按整数串发出，计价纠错缺评价交不出去', () => {
  const definition = settlementDecisionForm('charge-adjustment');
  const values = {
    ...initialGuidedValues(definition.fields),
    adjustment: 'ADJ',
    charge: 'CHG',
    kind: 'PRICING_CORRECTION',
    direction: 'DEBIT',
    originalCurrency: 'CNY',
    originalMinor: '9600',
    settlementCurrency: 'CNY',
    settlementMinor: '9600',
  };
  equal(buildGuidedPayload(definition, values).ok, false);
  const built = buildGuidedPayload(definition, { ...values, evaluation: 'EV' });
  equal(built.ok, true);
  if (built.ok) equal(built.payload.originalMinor, '9600');
  // 两种依据有此无彼：换过种类、上一种的引用还留着时不交。
  equal(buildGuidedPayload(definition, { ...values, evaluation: 'EV', authorization: 'AUTH' }).ok, false);
  equal(buildGuidedPayload(definition, { ...values, kind: 'COMMERCIAL_CONCESSION', evaluation: 'EV', authorization: 'AUTH' }).ok, false);
  equal(buildGuidedPayload(definition, { ...values, kind: 'COMMERCIAL_CONCESSION', authorization: 'AUTH' }).ok, true);
});

test('对账单发布不勾带走调整时不带这一格', () => {
  const definition = settlementDecisionForm('statement-publication');
  const built = buildGuidedPayload(definition, {
    ...initialGuidedValues(definition.fields),
    account: 'ACCT',
    period: '2026-10',
    version: 'v1',
    currency: 'CNY',
    charges: 'CHG-1',
    lastAttributionDate: '2026-10-31',
    declaredTotal: '9600',
    cutOffAt: '2026-10-31T16:00:00Z',
    carryUnbilledAdjustments: false,
  });
  equal(built.ok, true);
  if (built.ok) equal('carryUnbilledAdjustments' in built.payload, false);
});
