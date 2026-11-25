import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  codesPreview,
  countEntryRows,
  countNextStep,
  countProgress,
  countStatusLabel,
  countTrack,
  countWaitingSince,
  differenceDirection,
  differenceGap,
  differenceSummary,
  differenceTotals,
  effectiveEntryIds,
  lotLabel,
  mismatchedDifferences,
  signedGap,
  unitQuantities,
  type CountEntryLike,
  type DecidableDifference,
} from './count-flow';

test('状态词：待开始不用词表里通用的待处理，陌生的原样交回', () => {
  equal(countStatusLabel('PENDING'), '待开始');
  equal(countStatusLabel('COUNTING'), '盘点中');
  equal(countStatusLabel('COUNTED'), '已盘点');
  equal(countStatusLabel('VOID'), '已作废');
  equal(countStatusLabel('ARCHIVED'), 'ARCHIVED');
});

test('进度条：待开始卡在等人动手，盘点中正常往前走，已盘点走到末一步', () => {
  const pending = countTrack('PENDING', 0);
  equal(pending.currentId, 'PENDING');
  equal(pending.currentLabel, '待开始');

  const counting = countTrack('COUNTING', 0);
  equal(counting.currentId, 'COUNTING');
  equal(counting.currentLabel, undefined);
  equal(counting.halt, undefined);

  const counted = countTrack('COUNTED', 0);
  equal(counted.currentId, 'COUNTED');
  equal(counted.currentLabel, undefined);
});

test('已盘点而差异没批完：卡在批准这一步，写明还剩几条', () => {
  const counted = countTrack('COUNTED', 3);
  equal(counted.currentId, 'COUNTED');
  equal(counted.currentLabel, '已盘点，3 条差异待批');
});

test('作废是中性的停止标记，不当作走完了哪一步；陌生状态词原样示在第一步', () => {
  const voided = countTrack('VOID', 0);
  deepEqual(voided.halt, { label: '已作废', tone: 'neutral' });
  equal(voided.currentId, 'PENDING');
  equal(voided.currentLabel, undefined);

  const unknown = countTrack('ARCHIVED', 0);
  equal(unknown.currentId, 'PENDING');
  equal(unknown.currentLabel, 'ARCHIVED');
});

test('库位清点进度：缺哪几个按盘点范围里的次序，范围外的录入不算，重复的库位只算一次', () => {
  deepEqual(countProgress(['L-1', 'L-2', 'L-3'], ['L-2']), { total: 3, counted: 1, missing: ['L-1', 'L-3'] });
  deepEqual(countProgress(['L-1', 'L-2'], ['L-2', 'L-1', 'L-1']), { total: 2, counted: 2, missing: [] });
  deepEqual(countProgress(['L-1', 'L-1'], ['X-9']), { total: 1, counted: 0, missing: ['L-1'] });
  deepEqual(countProgress([], []), { total: 0, counted: 0, missing: [] });
});

const counting = { status: 'COUNTING', locations: ['L-1', 'L-2', 'L-3'], entryCount: 0, pendingDifferences: 0 };

test('下一步：待开始是开始', () => {
  deepEqual(countNextStep({ ...counting, status: 'PENDING' }), { kind: 'start', label: '开始盘点' });
});

test('盘点中笔数还不到库位数，必有库位没录，下一步是去作业台；够了才给结束', () => {
  deepEqual(countNextStep(counting), { kind: 'go', label: '去作业台录数', moduleId: 'warehouse-workstation' });
  deepEqual(countNextStep({ ...counting, entryCount: 2 }), { kind: 'go', label: '去作业台录数', moduleId: 'warehouse-workstation' });
  deepEqual(countNextStep({ ...counting, entryCount: 3 }), { kind: 'complete', label: '结束并出差异' });
  deepEqual(countNextStep({ ...counting, entryCount: 5 }), { kind: 'complete', label: '结束并出差异' });
});

test('已盘点的只有差异没批完才有下一步，写明几条；作废与陌生状态没有', () => {
  deepEqual(countNextStep({ ...counting, status: 'COUNTED', entryCount: 3, pendingDifferences: 2 }), { kind: 'review', label: '审批 2 条差异' });
  equal(countNextStep({ ...counting, status: 'COUNTED', entryCount: 3 }), null);
  equal(countNextStep({ ...counting, status: 'VOID' }), null);
  equal(countNextStep({ ...counting, status: 'ARCHIVED' }), null);
});

test('停留从进入当前这一步的那一刻算，已了结的不在等谁', () => {
  const base = { createdAt: '2026-11-20T08:00:00Z', startedAt: '2026-11-22T08:00:00Z', completedAt: '2026-11-24T08:00:00Z', pendingDifferences: 0 };
  equal(countWaitingSince({ ...base, status: 'PENDING' }), '2026-11-20T08:00:00Z');
  equal(countWaitingSince({ ...base, status: 'COUNTING' }), '2026-11-22T08:00:00Z');
  equal(countWaitingSince({ ...base, status: 'COUNTED', pendingDifferences: 1 }), '2026-11-24T08:00:00Z');
  equal(countWaitingSince({ ...base, status: 'COUNTED' }), undefined);
  equal(countWaitingSince({ ...base, status: 'VOID' }), undefined);
});

test('读口对没发生的时刻答空串：盘点中回落到创建时刻，已盘点的空串不当起点', () => {
  const base = { createdAt: '2026-11-20T08:00:00Z', startedAt: '', completedAt: '', pendingDifferences: 1 };
  equal(countWaitingSince({ ...base, status: 'COUNTING' }), '2026-11-20T08:00:00Z');
  equal(countWaitingSince({ ...base, status: 'COUNTED' }), undefined);
});

test('长串编码只写前几个，其余写个数', () => {
  equal(codesPreview(['L-1', 'L-2'], 3), 'L-1、L-2');
  equal(codesPreview(['L-1', 'L-2', 'L-3'], 3), 'L-1、L-2、L-3');
  equal(codesPreview(['L-1', 'L-2', 'L-3', 'L-4'], 3), 'L-1、L-2、L-3 等 4 个');
});

test('批号、效期、贴标有哪样写哪样，都没有答空串', () => {
  equal(lotLabel({}), '');
  equal(lotLabel({ batchNumber: 'B1' }), '批号 B1');
  equal(lotLabel({ batchNumber: 'B1', expiryDate: '2027-05-01', labelAlias: 'X-1' }), '批号 B1 · 效期 2027-05-01 · 贴标 X-1');
  equal(lotLabel({ batchNumber: '', expiryDate: '2027-05-01' }), '效期 2027-05-01');
});

// —— 账面与清点的比对 ——

const difference = (patch: Partial<DecidableDifference>): DecidableDifference => ({
  locationCode: 'L-1',
  goodsCode: 'G-1',
  unit: '件',
  status: 'PENDING',
  bookQuantity: 10,
  countedQuantity: 10,
  ...patch,
});

test('清点多于账面是盘盈，少于账面是盘亏，一致的不是差异', () => {
  equal(differenceGap({ bookQuantity: 10, countedQuantity: 13 }), 3);
  equal(differenceDirection({ bookQuantity: 10, countedQuantity: 13 }), 'gain');
  equal(differenceGap({ bookQuantity: 10, countedQuantity: 8 }), -2);
  equal(differenceDirection({ bookQuantity: 10, countedQuantity: 8 }), 'loss');
  equal(differenceDirection({ bookQuantity: 4, countedQuantity: 4 }), null);
  // 账上有、清点是 0 也是盘亏，整格没了。
  equal(differenceDirection({ bookQuantity: 6, countedQuantity: 0 }), 'loss');
  equal(differenceDirection({ bookQuantity: 0, countedQuantity: 6 }), 'gain');
});

test('差多少带正负号写', () => {
  equal(signedGap(3), '+3');
  equal(signedGap(-2), '-2');
  equal(signedGap(0), '0');
});

test('一致的行不混进差异；待批的排最前，其余按库位再按商品；不改传进来的数组', () => {
  const rows = [
    difference({ locationCode: 'L-2', goodsCode: 'G-1', status: 'APPROVED', countedQuantity: 12 }),
    difference({ locationCode: 'L-1', goodsCode: 'G-9', countedQuantity: 10 }),
    difference({ locationCode: 'L-3', goodsCode: 'G-1', status: 'PENDING', countedQuantity: 7 }),
    difference({ locationCode: 'L-1', goodsCode: 'G-2', status: 'REJECTED', countedQuantity: 9 }),
    difference({ locationCode: 'L-1', goodsCode: 'G-1', status: 'PENDING', countedQuantity: 11 }),
  ];
  const before = rows.map((row) => `${row.locationCode}/${row.goodsCode}`);
  const shown = mismatchedDifferences(rows);
  deepEqual(
    shown.map((row) => `${row.locationCode}/${row.goodsCode}/${row.status}`),
    ['L-1/G-1/PENDING', 'L-3/G-1/PENDING', 'L-1/G-2/REJECTED', 'L-2/G-1/APPROVED'],
  );
  deepEqual(rows.map((row) => `${row.locationCode}/${row.goodsCode}`), before);
});

test('合计只算不一致的行，盘盈与盘亏分开，待批单列', () => {
  const totals = differenceTotals([
    difference({ countedQuantity: 14 }),
    difference({ locationCode: 'L-2', countedQuantity: 13, status: 'APPROVED' }),
    difference({ locationCode: 'L-3', countedQuantity: 8 }),
    difference({ locationCode: 'L-4', countedQuantity: 10 }),
  ]);
  deepEqual(totals, { rows: 3, gainRows: 2, lossRows: 1, pendingRows: 2, byUnit: [{ unit: '件', gain: 7, loss: 2 }] });
  equal(differenceSummary(totals), '盘盈 2 行（+7 件） · 盘亏 1 行（-2 件）');
});

test('件与箱不跨单位合计：各单位各一栏', () => {
  const totals = differenceTotals([
    difference({ unit: '件', countedQuantity: 15 }),
    difference({ unit: '箱', countedQuantity: 12 }),
    difference({ unit: '箱', locationCode: 'L-2', bookQuantity: 5, countedQuantity: 1 }),
  ]);
  deepEqual(totals.byUnit, [
    { unit: '件', gain: 5, loss: 0 },
    { unit: '箱', gain: 2, loss: 4 },
  ]);
  equal(unitQuantities(totals.byUnit, 'gain'), '+5 件、+2 箱');
  equal(unitQuantities(totals.byUnit, 'loss'), '-4 箱');
  equal(differenceSummary(totals), '盘盈 2 行（+5 件、+2 箱） · 盘亏 1 行（-4 箱）');
});

test('没有差异时合计都是 0，摘要是空串', () => {
  const totals = differenceTotals([difference({}), difference({ locationCode: 'L-2' })]);
  deepEqual(totals, { rows: 0, gainRows: 0, lossRows: 0, pendingRows: 0, byUnit: [] });
  equal(differenceSummary(totals), '');
  deepEqual(differenceTotals([]), { rows: 0, gainRows: 0, lossRows: 0, pendingRows: 0, byUnit: [] });
});

// —— 清点录入 ——

const entry = (patch: Partial<CountEntryLike> & { factId: string }): CountEntryLike => ({
  locationCode: 'L-1',
  occurredAt: '2026-11-25T08:00:00Z',
  deviceSequence: 1,
  operatorId: 'OP-1',
  lines: [{ goodsCode: 'G-1', unit: '件', stockState: 'GOOD', quantity: 5 }],
  ...patch,
});

test('每个库位以最后一次清点为准：发生时刻晚的胜出，同一时刻按设备序号大的', () => {
  const entries = [
    entry({ factId: 'F-1', occurredAt: '2026-11-25T08:00:00Z' }),
    entry({ factId: 'F-2', occurredAt: '2026-11-25T09:00:00Z' }),
    entry({ factId: 'F-3', locationCode: 'L-2', occurredAt: '2026-11-25T08:00:00Z', deviceSequence: 4 }),
    entry({ factId: 'F-4', locationCode: 'L-2', occurredAt: '2026-11-25T08:00:00Z', deviceSequence: 9 }),
    entry({ factId: 'F-5', locationCode: 'L-3' }),
  ];
  deepEqual([...effectiveEntryIds(entries)].sort(), ['F-2', 'F-4', 'F-5']);
});

test('录入先到后到与读口给的次序无关：晚发生的先入册也是它胜出', () => {
  const entries = [
    entry({ factId: 'F-late', occurredAt: '2026-11-25T10:00:00Z' }),
    entry({ factId: 'F-early', occurredAt: '2026-11-25T07:00:00Z' }),
  ];
  deepEqual([...effectiveEntryIds(entries)], ['F-late']);
});

test('录入摊成一行一格货：同一库位挨在一起先录的在前，被取代的那一笔标出来', () => {
  const rows = countEntryRows([
    entry({ factId: 'F-new', locationCode: 'L-1', occurredAt: '2026-11-25T09:00:00Z', lines: [{ goodsCode: 'G-1', unit: '件', stockState: 'GOOD', quantity: 7 }] }),
    entry({ factId: 'F-other', locationCode: 'A-9' }),
    entry({ factId: 'F-old', locationCode: 'L-1', occurredAt: '2026-11-25T08:00:00Z' }),
  ]);
  deepEqual(
    rows.map((row) => [row.factId, row.locationCode, row.quantity, row.superseded]),
    [
      ['F-other', 'A-9', 5, false],
      ['F-old', 'L-1', 5, true],
      ['F-new', 'L-1', 7, false],
    ],
  );
});

test('一笔录入里有几行就摊几行，行里的空串当没有；一行都没有的是清点为空的库位', () => {
  const rows = countEntryRows([
    entry({
      factId: 'F-1',
      lines: [
        { goodsCode: 'G-1', unit: '件', stockState: 'GOOD', quantity: 2, batchNumber: 'B1', expiryDate: '', labelAlias: '' },
        { goodsCode: '', unit: '', stockState: '', quantity: 1, observed: '一箱没有标签的货' },
      ],
    }),
    entry({ factId: 'F-2', locationCode: 'L-2', lines: [] }),
  ]);
  equal(rows.length, 3);
  equal(rows[0].key, 'F-1-0');
  equal(rows[0].batchNumber, 'B1');
  equal(rows[0].expiryDate, undefined);
  equal(rows[0].empty, false);
  equal(rows[1].goodsCode, undefined);
  equal(rows[1].observed, '一箱没有标签的货');
  equal(rows[2].empty, true);
  equal(rows[2].quantity, 0);
  equal(rows[2].superseded, false);
});

test('时刻认不出的录入排在最早，不挤掉认得出的', () => {
  const entries = [
    entry({ factId: 'F-bad', occurredAt: '不是时刻', deviceSequence: 9 }),
    entry({ factId: 'F-good', occurredAt: '2026-11-25T08:00:00Z', deviceSequence: 1 }),
  ];
  deepEqual([...effectiveEntryIds(entries)], ['F-good']);
});
