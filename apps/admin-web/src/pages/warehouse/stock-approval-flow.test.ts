import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  adjustmentKindSign,
  adjustmentStatusLabel,
  adjustmentTotals,
  adjustmentTrack,
  discrepancyEffect,
  discrepancySignals,
  discrepancyStatusLabel,
  discrepancyTrack,
  kindQuantities,
  originLineIndex,
  signedAdjustmentQuantity,
} from './stock-approval-flow';

test('状态词：待批不用词表里通用的待处理，终点写已批准入账，陌生的原样交回', () => {
  equal(adjustmentStatusLabel('PENDING'), '待批');
  equal(adjustmentStatusLabel('POSTED'), '已批准入账');
  equal(adjustmentStatusLabel('REJECTED'), '已驳回');
  equal(adjustmentStatusLabel('ARCHIVED'), 'ARCHIVED');
  equal(discrepancyStatusLabel('PENDING'), '待批');
  equal(discrepancyStatusLabel('APPROVED'), '已批准入账');
  equal(discrepancyStatusLabel('REJECTED'), '已驳回');
});

test('调整的进度条：待批卡在等人批，批准走到入账，驳回是红色停止标记', () => {
  const pending = adjustmentTrack('PENDING');
  equal(pending.currentId, 'PENDING');
  equal(pending.currentLabel, '待批');
  equal(pending.halt, undefined);

  const posted = adjustmentTrack('POSTED');
  equal(posted.currentId, 'POSTED');
  equal(posted.currentLabel, undefined);

  const rejected = adjustmentTrack('REJECTED');
  deepEqual(rejected.halt, { label: '已驳回', tone: 'danger' });
  equal(rejected.currentId, 'PENDING');
  equal(rejected.currentLabel, undefined);
});

test('差异的进度条与调整同形，终点是 APPROVED；调整的终点词在差异上不算走完', () => {
  equal(discrepancyTrack('PENDING').currentLabel, '待批');
  equal(discrepancyTrack('APPROVED').currentId, 'APPROVED');
  deepEqual(discrepancyTrack('REJECTED').halt, { label: '已驳回', tone: 'danger' });
  const crossed = discrepancyTrack('POSTED');
  equal(crossed.currentId, 'PENDING');
  equal(crossed.currentLabel, 'POSTED');
});

test('陌生的状态词原样示在第一步，不猜它走到了哪', () => {
  const unknown = adjustmentTrack('ARCHIVED');
  equal(unknown.currentId, 'PENDING');
  equal(unknown.currentLabel, 'ARCHIVED');
});

test('调增是加，调减与销毁是减，不认识的种类不定方向', () => {
  equal(adjustmentKindSign('INCREASE'), '+');
  equal(adjustmentKindSign('DECREASE'), '-');
  equal(adjustmentKindSign('DESTRUCTION'), '-');
  equal(adjustmentKindSign('MYSTERY'), '');
  equal(signedAdjustmentQuantity({ kind: 'INCREASE', quantity: 5 }), '+5');
  equal(signedAdjustmentQuantity({ kind: 'DESTRUCTION', quantity: 2 }), '-2');
  equal(signedAdjustmentQuantity({ kind: 'MYSTERY', quantity: 2 }), '2');
});

const line = (patch: Partial<Parameters<typeof adjustmentTotals>[0][number]>) => ({
  kind: 'INCREASE',
  locationCode: 'L-1',
  goodsCode: 'G-1',
  unit: '件',
  quantity: 1,
  ...patch,
});

test('调整合计：行数、涉及商品与库位去重，种类按调增、调减、销毁排，只含有的种类', () => {
  const totals = adjustmentTotals([
    line({ kind: 'DESTRUCTION', quantity: 2 }),
    line({ kind: 'INCREASE', quantity: 5, goodsCode: 'G-2', locationCode: 'L-2' }),
    line({ kind: 'INCREASE', quantity: 3 }),
  ]);
  equal(totals.lines, 3);
  equal(totals.goods, 2);
  equal(totals.locations, 2);
  deepEqual(
    totals.kinds.map((kind) => [kind.kind, kind.rows, kindQuantities(kind)]),
    [
      ['INCREASE', 2, '+8 件'],
      ['DESTRUCTION', 1, '-2 件'],
    ],
  );
});

test('调整合计不跨单位：件与箱各写各的；不认识的种类排在后面也不丢行', () => {
  const totals = adjustmentTotals([
    line({ kind: 'DECREASE', quantity: 4 }),
    line({ kind: 'DECREASE', quantity: 1, unit: '箱' }),
    line({ kind: 'DECREASE', quantity: 2 }),
    line({ kind: 'MYSTERY', quantity: 7 }),
    line({ kind: 'INCREASE', quantity: 1 }),
  ]);
  deepEqual(
    totals.kinds.map((kind) => kind.kind),
    ['INCREASE', 'DECREASE', 'MYSTERY'],
  );
  equal(kindQuantities(totals.kinds[1]), '-6 件、-1 箱');
  equal(kindQuantities(totals.kinds[2]), '7 件');
  equal(totals.lines, 5);
});

test('没有行的调整合计都是 0', () => {
  deepEqual(adjustmentTotals([]), { lines: 0, goods: 0, locations: 0, kinds: [] });
});

test('差异的信号：账面短少、占用、整箱缺失各占一句，都没有答空数组', () => {
  deepEqual(discrepancySignals({ shortOnBook: 3, heldQuantity: 0, cartonMissing: false }), ['账面短少 3']);
  deepEqual(discrepancySignals({ shortOnBook: 3, heldQuantity: 2, cartonMissing: true }), ['账面短少 3', '占用 2', '整箱缺失']);
  deepEqual(discrepancySignals({ shortOnBook: 0, heldQuantity: 0, cartonMissing: false }), []);
});

test('批准会怎样按来源作业的种类说；读不到作业时写各种的总说，整箱缺失不论哪种作业都是同一句', () => {
  const plain = { cartonMissing: false };
  equal(discrepancyEffect('INTERNAL_MOVE', plain), '批准后，没入账的数量从来源库位补进目标库位。');
  equal(discrepancyEffect('SHORTAGE', plain), '批准后，在所在库位把短少的数量从账上调减。');
  equal(discrepancyEffect('FORM_CONVERSION', plain), discrepancyEffect('CONDITION_CONVERSION', plain));
  equal(discrepancyEffect('FORM_CONVERSION', plain).includes('转出'), true);
  const carton = discrepancyEffect('INTERNAL_MOVE', { cartonMissing: true });
  equal(carton.includes('整箱'), true);
  equal(carton, discrepancyEffect(undefined, { cartonMissing: true }));
  const general = discrepancyEffect(undefined, plain);
  for (const word of ['上架或移库', '短少', '形态与状态转换', '整箱缺失']) equal(general.includes(word), true, word);
  equal(discrepancyEffect('MYSTERY', plain), general);
});

test('差异形成它的作业行：按差异号认，认不到退到行位置，越界不猜', () => {
  const lines = [{ discrepancyId: undefined }, { discrepancyId: 'SDP-2' }, { discrepancyId: 'SDP-3' }];
  equal(originLineIndex(lines, 'SDP-3', 0), 2);
  equal(originLineIndex(lines, 'SDP-9', 1), 1);
  equal(originLineIndex(lines, 'SDP-9', 3), -1);
  equal(originLineIndex(lines, 'SDP-9', -1), -1);
  equal(originLineIndex(lines, 'SDP-9', 1.5), -1);
  equal(originLineIndex([], 'SDP-1', 0), -1);
});
