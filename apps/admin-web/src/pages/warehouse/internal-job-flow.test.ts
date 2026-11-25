import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  incompleteReasons,
  jobSignal,
  jobSummary,
  lineAccounting,
  lineLocations,
  lineOutcomeLabels,
  lineOutcomeTones,
  lineStory,
  plannedLabel,
  type JobLineFacts,
} from './internal-job-flow';

const goods = (patch: Partial<JobLineFacts>): JobLineFacts => ({
  ownerAccount: 'OWN-1',
  goodsCode: 'SKU-1',
  unit: '件',
  quantity: 10,
  sourceLocation: 'L-1',
  targetLocation: 'L-2',
  posted: 10,
  postedInbound: 10,
  shortOnBook: 0,
  held: 0,
  ...patch,
});

const carton = (patch: Partial<JobLineFacts>): JobLineFacts => ({
  ownerAccount: 'OWN-1',
  cartonNumber: 'CTN-1',
  targetLocation: 'L-2',
  posted: 1,
  postedInbound: 1,
  shortOnBook: 0,
  held: 0,
  ...patch,
});

test('商品行全部入账：入账数够计划数，没有没入的', () => {
  const acc = lineAccounting('INTERNAL_MOVE', goods({}));
  equal(acc.outcome, 'full');
  equal(acc.planned, 10);
  equal(acc.unposted, 0);
  equal(acc.unexplained, 0);
  equal(lineStory(acc), '全部入账');
});

test('账上不足：入了一部分，差的数全是账上本来就没有', () => {
  const acc = lineAccounting('INTERNAL_MOVE', goods({ posted: 6, postedInbound: 6, shortOnBook: 4 }));
  equal(acc.outcome, 'partial');
  equal(acc.unposted, 4);
  equal(acc.unexplained, 0);
  equal(lineStory(acc), '入账 6 / 计划 10：账上不足 4');
});

test('被占用挡住：账上有，冻结或预占占着取不走，一件也没入就是没入账', () => {
  const acc = lineAccounting('INTERNAL_MOVE', goods({ posted: 0, postedInbound: 0, held: 10 }));
  equal(acc.outcome, 'none');
  equal(acc.unposted, 10);
  equal(lineStory(acc), '计划 10，一件没入账：被占用挡住 10');
});

test('账上不足与被占用可以同时有：各写各的数', () => {
  const acc = lineAccounting('INTERNAL_MOVE', goods({ posted: 4, postedInbound: 4, shortOnBook: 4, held: 2 }));
  equal(acc.outcome, 'partial');
  equal(acc.unexplained, 0);
  equal(lineStory(acc), '入账 4 / 计划 10：账上不足 4，被占用挡住 2');
});

test('两类原因说不满没入的数时，余下的如实写成读口没说明，不硬凑进某一类', () => {
  const acc = lineAccounting('FORM_CONVERSION', goods({ posted: 7, postedInbound: 84, shortOnBook: 1, held: 0 }));
  equal(acc.unposted, 3);
  equal(acc.unexplained, 2);
  equal(lineStory(acc), '入账 7 / 计划 10：账上不足 1，另有 2 读口没说明原因');
  const none = lineAccounting('FORM_CONVERSION', goods({ posted: 0, postedInbound: 0 }));
  equal(none.unexplained, 10);
});

test('包装形态转换：已入账按转出单位、已入目标账按转入单位，两个数各是各的', () => {
  const acc = lineAccounting('FORM_CONVERSION', goods({ quantity: 2, posted: 2, postedInbound: 24 }));
  equal(acc.outcome, 'full');
  equal(acc.posted, 2);
  equal(acc.postedInbound, 24);
});

test('入账数超过计划数也算全部入账，不倒扣出没入的', () => {
  const acc = lineAccounting('INTERNAL_MOVE', goods({ posted: 12 }));
  equal(acc.outcome, 'full');
  equal(acc.unposted, 0);
});

test('计划数不是正数的商品行不算入账：没有什么可入', () => {
  equal(lineAccounting('INTERNAL_MOVE', goods({ quantity: 0, posted: 0 })).outcome, 'none');
  equal(lineAccounting('INTERNAL_MOVE', goods({ quantity: undefined, posted: 0 })).planned, 0);
});

test('整箱行：移动了是全部入账，不在账上是整箱缺失，在账上被占着是没移动', () => {
  const moved = lineAccounting('INTERNAL_MOVE', carton({}));
  equal(moved.outcome, 'full');
  equal(moved.planned, 1);
  equal(lineStory(moved), '整箱已移到目标库位');

  const missing = lineAccounting('INTERNAL_MOVE', carton({ posted: 0, postedInbound: 0, cartonMissing: true }));
  equal(missing.outcome, 'cartonMissing');
  equal(missing.unposted, 1);
  equal(missing.unexplained, 0);
  equal(lineStory(missing), '账上没有这一箱，没有移动');

  const held = lineAccounting('INTERNAL_MOVE', carton({ posted: 0, postedInbound: 0, held: 1 }));
  equal(held.outcome, 'none');
  equal(lineStory(held), '这一箱被冻结或占用着，没有移动');
});

test('短少上报不入账：不论数字怎么填，这一行只说已上报', () => {
  const acc = lineAccounting('SHORTAGE', goods({ posted: 0, postedInbound: 0, quantity: 3 }));
  equal(acc.outcome, 'reported');
  equal(acc.planned, 3);
  equal(acc.unposted, 0);
  equal(lineStory(acc), '已上报，不直接改账');
  equal(lineAccounting('SHORTAGE', carton({ posted: 0, postedInbound: 0 })).planned, 1);
});

test('每种结果有自己的词与色：全部入账绿、部分入账黄、没入账与整箱缺失红、上报中性', () => {
  equal(lineOutcomeLabels.full, '全部入账');
  equal(lineOutcomeTones.full, 'success');
  equal(lineOutcomeTones.partial, 'warning');
  equal(lineOutcomeTones.none, 'danger');
  equal(lineOutcomeTones.cartonMissing, 'danger');
  equal(lineOutcomeTones.reported, 'neutral');
});

test('计划数量连单位：整箱行是一箱，商品行写数量与来源单位，没有单位只写数量', () => {
  equal(plannedLabel(goods({ quantity: 10, unit: '件' })), '10 件');
  equal(plannedLabel(goods({ quantity: 10, unit: undefined })), '10');
  equal(plannedLabel(goods({ quantity: undefined })), '0 件');
  equal(plannedLabel(carton({})), '1 箱');
  equal(plannedLabel(carton({ cartonMissing: true, cartonNumber: undefined })), '1 箱');
});

test('库位：两头不同写来源到目标，两头一样（原地转换）与只有一头的只写一个，都没有写破折号', () => {
  equal(lineLocations({ sourceLocation: 'L-1', targetLocation: 'L-2' }), 'L-1 → L-2');
  equal(lineLocations({ sourceLocation: 'L-1', targetLocation: 'L-1' }), 'L-1');
  equal(lineLocations({ sourceLocation: 'L-1' }), 'L-1');
  equal(lineLocations({ targetLocation: 'L-2' }), 'L-2');
  equal(lineLocations({}), '—');
});

const mixed: JobLineFacts[] = [
  goods({}),
  goods({ ownerAccount: 'OWN-2', posted: 6, postedInbound: 6, shortOnBook: 4, discrepancyId: 'SDP-1' }),
  goods({ posted: 0, postedInbound: 0, held: 10, discrepancyId: 'SDP-2' }),
  carton({ posted: 0, postedInbound: 0, cartonMissing: true, discrepancyId: 'SDP-3' }),
  carton({}),
  goods({ posted: 5, postedInbound: 5, shortOnBook: 5, held: 0, discrepancyId: 'SDP-1' }),
];

test('一笔作业的合计：各种结果几行、整箱几行、涉及哪些货主、形成哪几条差异（去重、按行序）', () => {
  deepEqual(jobSummary('INTERNAL_MOVE', mixed), {
    lines: 6,
    cartonLines: 2,
    full: 2,
    partial: 2,
    none: 1,
    cartonMissing: 1,
    reported: 0,
    incomplete: 4,
    discrepancyIds: ['SDP-1', 'SDP-2', 'SDP-3'],
    owners: ['OWN-1', 'OWN-2'],
  });
});

test('短少上报的合计：每行都是已上报，没有没入完的，差异照样列出', () => {
  const summary = jobSummary('SHORTAGE', [goods({ discrepancyId: 'SDP-9', quantity: 2 }), carton({ discrepancyId: 'SDP-10' })]);
  equal(summary.reported, 2);
  equal(summary.incomplete, 0);
  equal(summary.cartonLines, 1);
  deepEqual(summary.discrepancyIds, ['SDP-9', 'SDP-10']);
});

test('没有行的作业合计都是 0', () => {
  deepEqual(jobSummary('INTERNAL_MOVE', []), {
    lines: 0,
    cartonLines: 0,
    full: 0,
    partial: 0,
    none: 0,
    cartonMissing: 0,
    reported: 0,
    incomplete: 0,
    discrepancyIds: [],
    owners: [],
  });
});

test('没入完的原因按原因数行：一行可以记进两类，整箱缺失单算，全部入账的行不进来', () => {
  deepEqual(incompleteReasons('INTERNAL_MOVE', mixed), ['账上不足 2 行', '被占用挡住 1 行', '整箱缺失 1 行']);
  deepEqual(incompleteReasons('INTERNAL_MOVE', [goods({ posted: 4, postedInbound: 4, shortOnBook: 4, held: 2 })]), ['账上不足 1 行', '被占用挡住 1 行']);
  deepEqual(incompleteReasons('INTERNAL_MOVE', [goods({})]), []);
  deepEqual(incompleteReasons('SHORTAGE', [goods({})]), []);
  deepEqual(incompleteReasons('FORM_CONVERSION', [goods({ posted: 0, postedInbound: 0 })]), ['原因读口没说明 1 行']);
});

test('列表上的入账信号：全部入账、几行没入完（提醒色）、短少上报几行、没有作业行', () => {
  deepEqual(jobSignal('INTERNAL_MOVE', [goods({}), carton({})]), { text: '全部入账', warn: false });
  deepEqual(jobSignal('INTERNAL_MOVE', mixed), { text: '4 / 6 行没入完', warn: true });
  deepEqual(jobSignal('SHORTAGE', [goods({}), goods({})]), { text: '2 行短少上报', warn: false });
  deepEqual(jobSignal('INTERNAL_MOVE', []), { text: '没有作业行', warn: false });
});
