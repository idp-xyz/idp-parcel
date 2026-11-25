import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { OutboundRequestDetailBody, OutboundRequestRecord } from '../api';
import { buildGuidedPayload } from '../guided-forms';
import { workFactPrefillFor, workFactSpecs } from '../work-fact';
import { prepareWorkFact } from './work-fact-submit';
import { confirmOffline, confirmParcel, handoffVerdict, orderOpened, scanCode, startDispatch } from './dispatch-flow';
import { handoffValues, judgeDispatch, sheetOf } from './dispatch-sheet';

const at = new Date('2026-11-25T08:30:15Z');

function detail(dispatch = 'PRODUCT_NETWORK', status = 'PACKED'): OutboundRequestDetailBody {
  const request: OutboundRequestRecord = {
    requestId: 'OBR-1',
    documentNumber: 'OB-1',
    ownerAccount: 'OWNER-1',
    warehouseCode: 'WH-1',
    origin: 'ORDER',
    requestKey: 'K',
    dispatch,
    recipient: '收件人',
    address: '地址',
    status,
    platformStocking: false,
    receivedAt: '2026-11-20T00:00:00Z',
    lines: [],
    reservedQuantity: 0,
    wantedQuantity: 1,
  };
  return {
    outcome: 'FOUND',
    request,
    parcels: [
      { parcelSequence: 1, packagingMaterial: 'BOX', weightValue: '1', weightUnit: 'KG', lengthValue: '1', widthValue: '1', heightValue: '1', dimensionUnit: 'CM', lines: [{ goodsCode: 'G-1', unit: '件', quantity: 1 }] },
      { parcelSequence: 2, packagingMaterial: 'BOX', weightValue: '1', weightUnit: 'KG', lengthValue: '1', widthValue: '1', heightValue: '1', dimensionUnit: 'CM', lines: [{ goodsCode: 'G-1', unit: '件', quantity: 1 }] },
    ],
    confirmations: [{ factId: 'F-1', parcelSequence: 1, kind: 'OUTBOUND_CONFIRMATION', offline: false, missing: '', occurredAt: '2026-11-25T00:00:00Z' }],
  };
}

function handing() {
  const opened = startDispatch('OBR-1');
  const sheet = sheetOf(detail());
  return orderOpened(opened.state, sheet, 'WH-1');
}

test('已确认过的包裹算交出；还没打包、别的仓各说各的', () => {
  const sheet = sheetOf(detail());
  if (!sheet) throw new Error('sheet');
  equal(sheet.parcels[0].done, true);
  equal(sheet.parcels[1].done, false);
  equal(judgeDispatch(sheet, 'WH-1').kind, 'ready');
  equal(judgeDispatch({ ...sheet, warehouse: 'WH-9' }, 'WH-1').kind, 'elsewhere');
  equal(judgeDispatch({ ...sheet, status: 'CHECKED' }, 'WH-1').kind, 'not-packed');
});

test('再扫这张单的号交出下一只包裹；离线是另一个按钮', () => {
  const open = handing();
  equal(open.state.phase, 'handing');
  equal(open.state.feedback.headline, '交出包裹 2');
  const scanned = scanCode(open.state, 'OB-1');
  equal(scanned.state.pending?.sequence, 2);
  equal(scanned.state.pending?.offline, false);
  const offline = confirmOffline(open.state);
  equal(offline.state.pending?.offline, true);
  if (offline.effects[0].kind !== 'submit') throw new Error('effect');
  equal(offline.effects[0].values.offline, true);
});

test('交出一只之后留在这张单上；都交完回到等扫', () => {
  const open = handing();
  const submitting = confirmParcel(open.state);
  const stayed = handoffVerdict(submitting.state, { verdict: 'done', outcome: 'RECORDED', headline: '已记录' });
  equal(stayed.state.phase, 'waiting');
  equal(stayed.state.feedback.headline, '这张单的包裹都已交出');
  const withheld = handoffVerdict(submitting.state, { verdict: 'refused', outcome: 'WITHHELD', headline: '放行条件还没满足' });
  equal(withheld.state.phase, 'handing');
  equal(withheld.state.sheet?.parcels[1].done, false);
});

test('出库确认的报文与旧作业台逐键相同；自提走另一个口', () => {
  const sheet = sheetOf(detail());
  if (!sheet) throw new Error('sheet');
  const values = handoffValues(sheet, 2, false);
  const spec = workFactSpecs.find((item) => item.id === 'outbound-confirmation');
  if (!spec) throw new Error('spec');
  const prepared = prepareWorkFact({ spec, deviceId: 'DEV-1', warehouse: 'WH-1', sequence: 3, values, now: at });
  if (!prepared.ok) throw new Error(prepared.reason);
  const legacy = buildGuidedPayload(spec.form, { ...workFactPrefillFor(spec, 'DEV-1', 'WH-1', 3, at), ...values });
  if (!legacy.ok) throw new Error('legacy');
  deepEqual(prepared.attempt.payload, { ...(spec.fixedPayload ?? {}), ...legacy.payload });
  equal('offline' in prepared.attempt.payload, false);
  const pickup = sheetOf(detail('PICKUP'));
  if (!pickup) throw new Error('pickup');
  equal(pickup.pickup, true);
  const pickupSpec = workFactSpecs.find((item) => item.id === 'pickup-handover');
  if (!pickupSpec) throw new Error('pickup spec');
  const pickupPrepared = prepareWorkFact({
    spec: pickupSpec,
    deviceId: 'DEV-1',
    warehouse: 'WH-1',
    sequence: 4,
    values: handoffValues(pickup, 2, true, '提货人', ''),
    now: at,
  });
  if (!pickupPrepared.ok) throw new Error(pickupPrepared.reason);
  equal(pickupPrepared.attempt.payload.offline, true);
  equal(pickupPrepared.attempt.payload.pickupParty, '提货人');
  equal('pickupCredential' in pickupPrepared.attempt.payload, false);
});
