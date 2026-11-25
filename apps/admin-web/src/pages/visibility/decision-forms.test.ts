import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { buildGuidedPayload, initialGuidedValues } from '../../components/guided-form/guided-forms';
import { exceptionDecisionEndpoints, type ExceptionDecisionKind } from './decisions';
import { exceptionDecisionForm, exceptionDecisionPrefill } from './decision-forms';

test('每个异常运营决定口都有逐格表单', () => {
  for (const kind of Object.keys(exceptionDecisionEndpoints) as ExceptionDecisionKind[]) {
    equal(exceptionDecisionForm(kind).fields.length > 0, true, kind);
  }
});

test('手工通知在信号三格之外还要方式和原件', () => {
  const definition = exceptionDecisionForm('customer-exception-notification-manual-record');
  const built = buildGuidedPayload(definition, {
    ...initialGuidedValues(definition.fields),
    parcel: 'P',
    signalKind: 'DELAY',
    episodeId: 'E',
    method: '电话',
    original: 'CALL-1',
  });
  equal(built.ok, true);
  if (built.ok) deepEqual(built.payload, { parcel: 'P', signalKind: 'DELAY', episodeId: 'E', method: '电话', original: 'CALL-1' });
});

test('选中的案件带进接单与关闭，不带进归并', () => {
  deepEqual(exceptionDecisionPrefill('CASE-1'), { 'case-take-up': { caseId: 'CASE-1' }, 'case-closure': { caseId: 'CASE-1' } });
  deepEqual(exceptionDecisionPrefill(null), {});
});
