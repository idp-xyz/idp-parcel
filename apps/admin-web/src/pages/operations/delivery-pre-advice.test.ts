import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { formatInstant } from '../catalogue-view';
import {
  deliveryScopeListPathOf,
  deliveryScopeReadProblemNote,
  localDayBoundsOf,
  preAdviceFilterKeys,
  preAdviceRowOf,
  handoverNoteFilterKeys,
} from './delivery-pre-advice';
import type { DeliveryPreAdviceRecord } from './delivery-scope-api';

// 送站预报区（票 ps-external-mark-relations/13「做什么」第 1 项）。最容易错的是「预计送达日」：读口只收瞬间，一日从哪一刻起算随看的人
// 所在的时区，页面把日子换成那一日本地零点到次日零点的左闭右开区间；换错一格，零点那一份就落进前一日。

function preAdvice(overrides: Partial<DeliveryPreAdviceRecord> = {}): DeliveryPreAdviceRecord {
  return {
    preAdviceId: 'SYN-PREADV-PSXM13-1',
    customerAccount: 'SYN-ACCOUNT-01',
    node: 'SYN-NODE-PSXM13',
    expectedArrivalAt: '2026-10-20T10:00:00Z',
    customerParcelNumbers: ['SYN-NUMBER-2', 'SYN-NUMBER-1'],
    formedAt: '2026-10-19T08:00:00Z',
    withdrawn: false,
    ...overrides,
  };
}

test('预计送达日：换成本地零点到次日本地零点，左闭右开', () => {
  const bounds = localDayBoundsOf('2026-10-20');
  ok(bounds);
  equal(bounds.from, new Date(2026, 9, 20).toISOString());
  equal(bounds.until, new Date(2026, 9, 21).toISOString());
  // 月末与年末照样进到下一日。
  equal(localDayBoundsOf('2026-12-31')?.until, new Date(2027, 0, 1).toISOString());
  for (const bad of ['', '2026-10', '2026-13-01', '2026-02-30', '20261020', 'yesterday']) {
    equal(localDayBoundsOf(bad), null, bad);
  }
});

test('列表路径：只带填了的格；节点去两端空白；日子换成两端瞬间；两本册各用各的键', () => {
  equal(deliveryScopeListPathOf('/node-operations-delivery-pre-advices', preAdviceFilterKeys, { node: '', day: '' }), '/node-operations-delivery-pre-advices');
  equal(
    deliveryScopeListPathOf('/node-operations-delivery-pre-advices', preAdviceFilterKeys, { node: ' SYN-NODE-1 ', day: '' }),
    '/node-operations-delivery-pre-advices?node=SYN-NODE-1',
  );
  const bounds = localDayBoundsOf('2026-10-20');
  ok(bounds);
  equal(
    deliveryScopeListPathOf('/node-operations-delivery-handover-notes', handoverNoteFilterKeys, { node: 'SYN NODE', day: '2026-10-20' }),
    `/node-operations-delivery-handover-notes?node=SYN+NODE&openedFrom=${encodeURIComponent(bounds.from)}&openedUntil=${encodeURIComponent(bounds.until)}`,
  );
  equal(
    deliveryScopeListPathOf('/node-operations-delivery-pre-advices', preAdviceFilterKeys, { node: '', day: '2026-10-20' }),
    `/node-operations-delivery-pre-advices?expectedArrivalFrom=${encodeURIComponent(bounds.from)}&expectedArrivalUntil=${encodeURIComponent(bounds.until)}`,
  );
  // 认不出的日子不发半截条件：宁可不筛，也不把「坏日子」读成「这一日」。
  equal(deliveryScopeListPathOf('/node-operations-delivery-pre-advices', preAdviceFilterKeys, { node: '', day: '2026-02-30' }), null);
});

test('预报行：撤回与否、号码数、交付方线索缺席各写各的字', () => {
  deepEqual(preAdviceRowOf(preAdvice({ deliveringParty: 'SYN-COURIER-1' })), {
    key: 'pre-advice:SYN-PREADV-PSXM13-1',
    preAdviceId: 'SYN-PREADV-PSXM13-1',
    values: {
      preAdviceId: 'SYN-PREADV-PSXM13-1',
      customerAccount: 'SYN-ACCOUNT-01',
      node: 'SYN-NODE-PSXM13',
      expectedArrivalAt: formatInstant('2026-10-20T10:00:00Z'),
      numbers: '2 个',
      deliveringParty: 'SYN-COURIER-1',
      withdrawal: '未撤回',
      formedAt: formatInstant('2026-10-19T08:00:00Z'),
    },
  });
  const withdrawn = preAdviceRowOf(preAdvice({ withdrawn: true, withdrawnAt: '2026-10-19T18:00:00Z' }));
  equal(withdrawn.values.withdrawal, `已撤回 · ${formatInstant('2026-10-19T18:00:00Z')}`);
  equal(withdrawn.values.deliveringParty, '（没给交付方线索）');
});

test('读口问题码：看不见、坏筛选与没形成答案各有一句', () => {
  ok(deliveryScopeReadProblemNote('DELIVERY_PRE_ADVICE_NOT_VISIBLE').includes('查无'));
  ok(deliveryScopeReadProblemNote('DELIVERY_HANDOVER_NOTE_NOT_VISIBLE').includes('查无'));
  ok(deliveryScopeReadProblemNote('MALFORMED_REQUEST').includes('筛选'));
  ok(deliveryScopeReadProblemNote('NO_ANSWER_FORMED').includes('重试'));
  ok(deliveryScopeReadProblemNote('SOMETHING_NEW').includes('SOMETHING_NEW'));
});
