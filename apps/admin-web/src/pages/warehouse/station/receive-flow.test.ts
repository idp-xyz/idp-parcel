import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { PreAdviceRecord } from '../api';
import { buildGuidedPayload } from '../guided-forms';
import { workFactPrefillFor, workFactSpecs } from '../work-fact';
import { prepareWorkFact } from './work-fact-submit';
import {
  acceptStray,
  adviceFound,
  chooseStockState,
  confirmAsk,
  goodsIdentified,
  locationVerdict,
  receiptVerdict,
  registerStray,
  scanCode,
  startReceive,
  submitReceipt,
  type ReceiveState,
} from './receive-flow';
import { goodsFactValues, judgeAdvice, sheetOf, unclaimedFactValues } from './receive-sheet';

const now = new Date('2026-11-25T12:00:00Z');
const at = new Date('2026-11-25T08:30:15Z');

function record(patch: Partial<PreAdviceRecord> = {}): PreAdviceRecord {
  return {
    preAdviceId: 'IPA-1',
    ownerAccount: 'OWNER-1',
    warehouseCode: 'WH-1',
    arrivalMode: 'APPOINTMENT',
    inboundMode: 'BY_GOODS',
    status: 'ADVISED',
    documentNumber: 'IB-1',
    submissionPath: 'OWNER',
    registrant: 'OWNER-1',
    acceptedAt: '2026-11-20T00:00:00Z',
    goodsLines: [{ goodsCode: 'G-1', unit: '件', quantity: 10 }],
    currentNotice: { version: 1, expectedAt: '2026-11-24T00:00:00Z', submissionPath: 'OWNER', registrant: 'OWNER-1', recordedAt: '2026-11-20T00:00:00Z' },
    cumulativeReceived: [{ goodsCode: 'G-1', quantity: 4 }],
    ...patch,
  };
}

function sheet() {
  const made = sheetOf(record(), now);
  if ('kind' in made) throw new Error(made.kind);
  return made;
}

function atGoods(state: ReceiveState): ReceiveState {
  const found = adviceFound(state, { kind: 'sheet', sheet: sheet() }, 'WH-1');
  const located = locationVerdict(found.state, 'LOC-R', { kind: 'ok' });
  return chooseStockState(located.state, 'GOOD').state;
}

test('预报：已迟只在已预报且预计时刻已过时写；已收数量从累计里来', () => {
  const open = sheet();
  equal(open.late.startsWith('已迟') || open.late.endsWith('天') || open.late.includes('天'), true);
  equal(open.goods[0].received, 4);
  const receiving = sheetOf(record({ status: 'RECEIVING' }), now);
  if ('kind' in receiving) throw new Error('mode');
  equal(receiving.late, '');
  equal(judgeAdvice(open, 'WH-1').kind, 'receive');
  equal(judgeAdvice(open, 'WH-2').kind, 'elsewhere');
  equal(judgeAdvice({ ...open, status: 'CANCELLED' }, 'WH-1').kind, 'divert');
  equal(judgeAdvice({ ...open, status: 'CLOSED' }, 'WH-1').kind, 'divert');
});

test('扫单号、核暂存位、选状态之后才收货；状态不预选', () => {
  const started = startReceive(null, null);
  equal(started.state.phase, 'waiting');
  equal(started.state.stockState, null);
  equal(startReceive(null, null, 'GOOD').state.stockState, 'GOOD');
  equal(startReceive(null, null, 'QUARANTINE').state.stockState, null);
  const finding = scanCode(started.state, 'IB-1');
  equal(finding.state.phase, 'finding');
  equal(finding.effects[0].kind, 'find');
  const located = adviceFound(finding.state, { kind: 'sheet', sheet: sheet() }, 'WH-1');
  equal(located.state.phase, 'locating');
  const verifying = scanCode(located.state, 'LOC-R');
  equal(verifying.state.phase, 'verifying');
  const verified = locationVerdict(verifying.state, 'LOC-R', { kind: 'ok' });
  equal(verified.state.phase, 'state');
  deepEqual(verified.effects[0], { kind: 'remember-location', code: 'LOC-R' });
  const chosen = chooseStockState(verified.state, 'PENDING_INSPECTION');
  equal(chosen.state.phase, 'goods');
  equal(chosen.state.stockState, 'PENDING_INSPECTION');
});

test('别的仓、没有、没核成各说各的，没核成不说成没有', () => {
  const finding = scanCode(startReceive(null, null).state, 'NOPE');
  const elsewhere = adviceFound(finding.state, { kind: 'sheet', sheet: { ...sheet(), warehouseCode: 'WH-9' } }, 'WH-1');
  equal(elsewhere.state.feedback.headline, '这张预报不在这个仓');
  const missing = adviceFound(finding.state, { kind: 'absent' }, 'WH-1');
  equal(missing.state.feedback.headline, '没有这张预报');
  const unread = adviceFound(finding.state, { kind: 'unread', reason: '读预报没成' }, 'WH-1');
  equal(unread.state.feedback.headline, '这一次没核成');
  equal(unread.state.feedback.headline.includes('没有'), false);
});

test('扫到预报上的货加一；要批号时先追问，不填不记', () => {
  const goods = atGoods(scanCode(startReceive('LOC-R', 'IPA-1').state, 'x').state);
  const scanned = scanCode(goods, 'G-1');
  deepEqual(scanned.effects[0], { kind: 'identify', owner: 'OWNER-1', code: 'G-1' });
  const counted = goodsIdentified(scanned.state, 'G-1', { kind: 'found', goodsCode: 'G-1', unit: '件', trackBatch: false, trackExpiry: false });
  equal(counted.state.goods[0].quantity, 1);
  const again = goodsIdentified(scanCode(counted.state, 'G-1').state, 'G-1', { kind: 'found', goodsCode: 'G-1', unit: '件' });
  equal(again.state.goods[0].quantity, 2);
  const asking = goodsIdentified(scanCode(counted.state, 'G-1').state, 'G-1', { kind: 'found', goodsCode: 'G-1', unit: '件', trackBatch: true });
  equal(asking.state.phase, 'asking');
  equal(confirmAsk(asking.state, '', '', '件').state.phase, 'asking');
  const asked = confirmAsk(asking.state, 'B-9', '', '件').state.goods;
  equal(asked[asked.length - 1]?.batchNumber, 'B-9');
});

test('认不出不记数；照收进报文，登记待认领是另一笔', () => {
  const goods = atGoods(scanCode(startReceive('LOC-R', null).state, 'IB-1').state);
  const stray = goodsIdentified(scanCode(goods, 'UNKNOWN').state, 'UNKNOWN', { kind: 'unknown' });
  equal(stray.state.goods.length, 0);
  equal(stray.state.stray, 'UNKNOWN');
  const asking = acceptStray(stray.state, { kind: 'unknown' });
  const taken = confirmAsk(asking.state, '', '', '件');
  equal(taken.state.goods[0].goodsCode, null);
  const registered = registerStray(stray.state);
  equal(registered.effects[0].kind, 'submit');
  if (registered.effects[0].kind === 'submit') {
    equal(registered.effects[0].spec, 'unclaimed-arrival');
    equal(registered.effects[0].values.kind, 'INBOUND');
  }
});

test('收货报文与旧作业台的定义逐键相同；待认领带发生时刻和设备序号', () => {
  const advice = sheet();
  const values = goodsFactValues(advice, 'LOC-R', 'GOOD', [{ observed: 'G-1', goodsCode: 'G-1', unit: '件', quantity: 6 }]);
  const spec = workFactSpecs.find((item) => item.id === 'receipt-goods');
  if (!spec) throw new Error('spec');
  const prepared = prepareWorkFact({ spec, deviceId: 'DEV-1', warehouse: 'WH-1', sequence: 3, values, now: at });
  if (!prepared.ok) throw new Error(prepared.reason);
  const legacy = buildGuidedPayload(spec.form, { ...workFactPrefillFor(spec, 'DEV-1', 'WH-1', 3, at), ...values });
  if (!legacy.ok) throw new Error('legacy');
  deepEqual(prepared.attempt.payload, { ...(spec.fixedPayload ?? {}), ...legacy.payload });
  equal(prepared.attempt.payload.shape, 'BY_GOODS');
  const unclaimed = workFactSpecs.find((item) => item.id === 'unclaimed-arrival');
  if (!unclaimed) throw new Error('unclaimed');
  const other = prepareWorkFact({
    spec: unclaimed,
    deviceId: 'DEV-1',
    warehouse: 'WH-1',
    sequence: 4,
    values: unclaimedFactValues(advice, 'UNKNOWN', 'LOC-R'),
    now: at,
  });
  if (!other.ok) throw new Error(other.reason);
  equal(other.attempt.payload.kind, 'INBOUND');
  equal(typeof other.attempt.payload.deviceSequence, 'number');
  equal(typeof other.attempt.payload.occurredAt, 'string');
});

test('提交成功回到等扫单号；没送到留着再送；转待认领照实说', () => {
  const goods = atGoods(scanCode(startReceive('LOC-R', null).state, 'IB-1').state);
  const counted = goodsIdentified(scanCode(goods, 'G-1').state, 'G-1', { kind: 'found', goodsCode: 'G-1', unit: '件' });
  const submitting = submitReceipt(counted.state);
  equal(submitting.state.phase, 'submitting');
  const done = receiptVerdict(submitting.state, { verdict: 'done', outcome: 'CONFIRMED', headline: '已确认并入账' }, 'receipt-goods');
  equal(done.state.phase, 'waiting');
  equal(done.state.location, 'LOC-R');
  equal(done.state.stockState, 'GOOD');
  const diverted = receiptVerdict(submitting.state, { verdict: 'done', outcome: 'DIVERTED_TO_UNCLAIMED', headline: '已转待认领' }, 'receipt-goods');
  equal(diverted.state.feedback.headline, '没有入账，转成了待认领');
  const unsent = receiptVerdict(submitting.state, { verdict: 'retry', headline: '没送到' }, 'receipt-goods');
  equal(unsent.state.phase, 'unsent');
});

test('核库位没问成不记这个库位', () => {
  const finding = scanCode(startReceive(null, null).state, 'IB-1');
  const locating = adviceFound(finding.state, { kind: 'sheet', sheet: sheet() }, 'WH-1');
  const unverified = locationVerdict(scanCode(locating.state, 'LOC-X').state, 'LOC-X', { kind: 'unverified', reason: '读库位名册没成' });
  equal(unverified.state.phase, 'locating');
  equal(unverified.state.location, null);
  equal(unverified.state.feedback.headline, '这一次没核成');
});
