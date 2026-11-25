import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { buildGuidedPayload } from '../guided-forms';
import { workFactPrefillFor, workFactSpecs } from '../work-fact';
import { prepareWorkFact } from './work-fact-submit';
import { chooseStockState, countOpened, countValues, goodsFound, markUnknown, scanCode, startCount, submitLocation, type CountSheet } from './count-flow';

const at = new Date('2026-11-25T08:30:15Z');
const sheet: CountSheet = { countId: 'CNT-1', warehouse: 'WH-1', status: 'COUNTING', owner: 'OWNER-1', locations: ['L-1', 'L-2'], counted: ['L-1'] };

function atGoods() {
  const opened = countOpened(startCount('CNT-1').state, sheet, 'WH-1');
  const located = scanCode(opened.state, 'L-2');
  return chooseStockState(located.state, 'GOOD').state;
}

test('不在范围里的库位不记；点过的库位说明再提交以这一笔为准', () => {
  const opened = countOpened(startCount(null).state, sheet, 'WH-1');
  equal(opened.state.phase, 'location');
  const outside = scanCode(opened.state, 'L-9');
  equal(outside.state.phase, 'location');
  equal(outside.state.feedback.headline, '这个库位不在这张盘点单里');
  const again = scanCode(opened.state, 'L-1');
  equal(again.state.phase, 'goods');
  equal(again.state.feedback.detail.includes('以这一笔为准'), true);
});

test('没选状态不记货；认得出的加一，认不出的不记，点了才按观察记', () => {
  const opened = countOpened(startCount('CNT-1').state, sheet, 'WH-1');
  const located = scanCode(opened.state, 'L-2');
  equal(scanCode(located.state, 'G-1').effects.length, 0);
  const chosen = chooseStockState(located.state, 'PENDING_INSPECTION');
  const found = goodsFound(scanCode(chosen.state, 'G-1').state, 'G-1', { goodsCode: 'G-1', unit: '件' });
  equal(found.state.lines[0].quantity, 1);
  equal(found.state.lines[0].stockState, 'PENDING_INSPECTION');
  const unknown = goodsFound(scanCode(found.state, 'NOPE').state, 'NOPE', null);
  equal(unknown.state.lines.length, 1);
  const marked = markUnknown(unknown.state);
  equal(marked.state.lines[1].observed, 'NOPE');
  equal(marked.state.lines[1].goodsCode, undefined);
});

test('空库位提交空数组；报文与旧作业台逐键相同', () => {
  const state = atGoods();
  const empty = submitLocation(state, true);
  if (empty.effects[0].kind !== 'submit') throw new Error('effect');
  deepEqual(empty.effects[0].values.lines, []);
  const values = countValues('CNT-1', 'L-2', [{ ownerAccount: 'OWNER-1', goodsCode: 'G-1', unit: '件', stockState: 'GOOD', quantity: 2 }]);
  const spec = workFactSpecs.find((item) => item.id === 'count-entry');
  if (!spec) throw new Error('spec');
  const prepared = prepareWorkFact({ spec, deviceId: 'DEV-1', warehouse: 'WH-1', sequence: 4, values, now: at });
  if (!prepared.ok) throw new Error(prepared.reason);
  const legacy = buildGuidedPayload(spec.form, { ...workFactPrefillFor(spec, 'DEV-1', 'WH-1', 4, at), ...values });
  if (!legacy.ok) throw new Error('legacy');
  deepEqual(prepared.attempt.payload, { ...(spec.fixedPayload ?? {}), ...legacy.payload });
  equal(prepared.attempt.payload.deviceSequence, 4);
});
