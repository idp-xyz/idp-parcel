import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { adviceActionPrefill, adviceHeading, claimAdvicePayload, headingOf } from './advice-action';

const advised = {
  preAdviceId: 'IPA-1',
  documentNumber: 'IB-20261127-00002',
  status: 'ADVISED',
  inboundMode: 'BY_GOODS',
  goodsLines: [{ goodsCode: 'SYN-GOODS-01', unit: '件', quantity: 6 }],
  cumulativeReceived: [],
};

test('有单据号时抬头用单据号，完结带上已收和预报', () => {
  deepEqual(adviceHeading(advised, false), { title: 'IB-20261127-00002', detail: '已预报' });
  deepEqual(adviceHeading({ ...advised, status: 'RECEIVING', cumulativeReceived: [{ goodsCode: 'SYN-GOODS-01', quantity: 1 }] }, true), {
    title: 'IB-20261127-00002',
    detail: '收货中 · 已收 1 / 预报 6',
  });
});

test('还没编出单据号时抬头标明内部标识', () => {
  deepEqual(adviceHeading({ ...advised, documentNumber: '  ' }, false), { title: '内部标识 IPA-1', detail: '已预报' });
});

test('预填把预报标识和抬头分开，抬头读得回来', () => {
  const prefill = adviceActionPrefill(advised, false);
  equal(prefill.preAdviceId, 'IPA-1');
  deepEqual(headingOf(prefill), { title: 'IB-20261127-00002', detail: '已预报' });
  equal(headingOf({ preAdviceId: 'IPA-1' }), null);
});

test('认领按单据号换成预报标识；对不上就留下原文；读口没答上来不送', () => {
  const payload = { arrivalId: 'UCA-1', preAdviceId: 'IB-20261127-00002' };
  deepEqual(claimAdvicePayload(payload, { ok: true, found: true, preAdviceId: 'IPA-1' }), {
    ok: true,
    payload: { arrivalId: 'UCA-1', preAdviceId: 'IPA-1' },
  });
  deepEqual(claimAdvicePayload(payload, { ok: true, found: false }), { ok: true, payload });
  deepEqual(claimAdvicePayload(payload, { ok: false }), { ok: false, message: '没有问到这张入库单，请重试。' });
  deepEqual(claimAdvicePayload({ arrivalId: 'UCA-1' }, { ok: false }), { ok: true, payload: { arrivalId: 'UCA-1' } });
});
