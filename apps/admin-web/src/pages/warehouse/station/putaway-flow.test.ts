import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { buildGuidedPayload } from '../guided-forms';
import { workFactPrefillFor, workFactSpecs } from '../work-fact';
import { prepareWorkFact } from './work-fact-submit';
import { chooseOwner, moveValues, placeVerdict, scanCode, startPutaway, stockLoaded, type StockCell } from './putaway-flow';

const at = new Date('2026-11-25T08:30:15Z');
const cell: StockCell = { goodsCode: 'G-1', unit: '件', stockState: 'GOOD', quantity: 4, inboundBatchId: 'B-1' };

function ready() {
  const owned = chooseOwner(startPutaway().state, 'OWNER-1');
  const verifying = scanCode(owned.state, 'LOC-A');
  const placed = placeVerdict(verifying.state, 'LOC-A', { kind: 'ok', usage: 'RECEIVING_STAGING' });
  return stockLoaded(placed.state, [cell], []).state;
}

test('没选货主不收扫码；源库位核过才扫货', () => {
  equal(scanCode(startPutaway().state, 'LOC-A').state.phase, 'owner');
  const state = ready();
  equal(state.phase, 'goods');
  equal(state.source, 'LOC-A');
});

test('扫货加一；扫一个库位码就当目标并提交；次品库区先说明', () => {
  const once = scanCode(ready(), 'G-1');
  equal(once.state.draft?.quantity, 1);
  const twice = scanCode(once.state, 'G-1');
  equal(twice.state.draft?.quantity, 2);
  const moving = scanCode(twice.state, 'LOC-B');
  equal(moving.state.phase, 'verifying');
  const submitted = placeVerdict(moving.state, 'LOC-B', { kind: 'ok', usage: 'STORAGE' });
  equal(submitted.state.phase, 'submitting');
  equal(submitted.effects[0].kind, 'submit');
  const defective = placeVerdict(moving.state, 'LOC-D', { kind: 'ok', usage: 'DEFECTIVE' });
  equal(defective.state.feedback.headline.includes('次品'), true);
});

test('库位没核成不记', () => {
  const owned = chooseOwner(startPutaway().state, 'OWNER-1');
  const verifying = scanCode(owned.state, 'NOPE');
  const unread = placeVerdict(verifying.state, 'NOPE', { kind: 'unverified', reason: '读库位名册没成' });
  equal(unread.state.phase, 'source');
  equal(unread.state.source, null);
  equal(unread.state.feedback.headline, '这一次没核成');
});

test('移动报文与旧作业台的定义逐键相同，不带折合', () => {
  const submitted = placeVerdict(scanCode(scanCode(ready(), 'G-1').state, 'LOC-B').state, 'LOC-B', { kind: 'ok', usage: 'STORAGE' });
  if (submitted.effects[0].kind !== 'submit') throw new Error('effect');
  const values = submitted.effects[0].values;
  deepEqual(moveValues(submitted.state).lines, values.lines);
  const spec = workFactSpecs.find((item) => item.id === 'internal-move');
  if (!spec) throw new Error('spec');
  const prepared = prepareWorkFact({ spec, deviceId: 'DEV-1', warehouse: 'WH-1', sequence: 3, values, now: at });
  if (!prepared.ok) throw new Error(prepared.reason);
  const legacy = buildGuidedPayload(spec.form, { ...workFactPrefillFor(spec, 'DEV-1', 'WH-1', 3, at), ...values });
  if (!legacy.ok) throw new Error('legacy');
  deepEqual(prepared.attempt.payload, { ...(spec.fixedPayload ?? {}), ...legacy.payload });
  const line = (prepared.attempt.payload.lines as Array<Record<string, unknown>>)[0];
  equal(line.ownerAccount, 'OWNER-1');
  equal(line.quantity, 1);
  equal('toUnit' in line, false);
  equal(line.inboundBatchId, 'B-1');
});
