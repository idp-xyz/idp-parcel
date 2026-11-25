import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  instructionGrantLabel,
  instructionHeadline,
  instructionMaterials,
  instructionNextStep,
  instructionShortfall,
  instructionStatusLabel,
  instructionTitle,
  instructionTrack,
  instructionVoidActionId,
  instructionWaitingSince,
  relabelPlan,
  type InstructionFacts,
} from './instruction-flow';

test('状态词：待执行、已执行、已作废；已执行不用通用词表里的已完成，陌生的原样交回', () => {
  equal(instructionStatusLabel('PENDING'), '待执行');
  equal(instructionStatusLabel('DONE'), '已执行');
  equal(instructionStatusLabel('VOID'), '已作废');
  equal(instructionStatusLabel('ARCHIVED'), 'ARCHIVED');
});

test('进度条：待执行卡在等现场、用待办色；执行走到末一步；作废是中性的停止标记，步子都不算走过', () => {
  const pending = instructionTrack('PENDING');
  equal(pending.currentId, 'PENDING');
  equal(pending.currentLabel, '待执行');
  equal(pending.halt, undefined);

  const done = instructionTrack('DONE');
  equal(done.currentId, 'DONE');
  equal(done.currentLabel, undefined);
  equal(done.halt, undefined);

  const voided = instructionTrack('VOID');
  deepEqual(voided.halt, { label: '已作废', tone: 'neutral' });
  equal(voided.currentId, 'PENDING');
  equal(voided.currentLabel, undefined);
});

test('陌生的状态词原样示在第一步，不猜它走到了哪', () => {
  const unknown = instructionTrack('ARCHIVED');
  equal(unknown.currentId, 'PENDING');
  equal(unknown.currentLabel, 'ARCHIVED');
});

test('下一步只有待执行的有：去作业台执行；已执行与已作废没有，陌生状态不替它猜', () => {
  deepEqual(instructionNextStep('PENDING'), { label: '去作业台执行', moduleId: 'warehouse-workstation' });
  equal(instructionNextStep('DONE'), null);
  equal(instructionNextStep('VOID'), null);
  equal(instructionNextStep('ARCHIVED'), null);
});

test('等了多久从受理算，只有待执行的在等', () => {
  equal(instructionWaitingSince({ status: 'PENDING', receivedAt: '2026-11-25T01:00:00Z' }), '2026-11-25T01:00:00Z');
  equal(instructionWaitingSince({ status: 'DONE', receivedAt: '2026-11-25T01:00:00Z' }), undefined);
  equal(instructionWaitingSince({ status: 'VOID', receivedAt: '2026-11-25T01:00:00Z' }), undefined);
});

test('作废走哪个动作与授予按种类分：换标一个口，加工与拆解共一个口', () => {
  equal(instructionVoidActionId('RELABEL'), 'relabel-void');
  equal(instructionVoidActionId('PROCESS'), 'processing-void');
  equal(instructionVoidActionId('DISASSEMBLE'), 'processing-void');
  equal(instructionGrantLabel('RELABEL'), '换标指令');
  equal(instructionGrantLabel('PROCESS'), '加工指令');
  equal(instructionGrantLabel('DISASSEMBLE'), '加工指令');
});

test('单据页的种类叫法：三种各叫各的，读不到种类时只叫指令', () => {
  equal(instructionTitle('PROCESS'), '加工指令');
  equal(instructionTitle('DISASSEMBLE'), '拆解指令');
  equal(instructionTitle('RELABEL'), '换标指令');
  equal(instructionTitle(undefined), '指令');
  equal(instructionTitle('MYSTERY'), '指令');
});

const base: InstructionFacts = {
  kind: 'PROCESS',
  goodsCode: 'KIT-1',
  unit: '套',
  quantity: 10,
  doneQuantity: 0,
  components: [
    { goodsCode: 'PART-A', quantity: 2 },
    { goodsCode: 'PART-B', quantity: 1 },
  ],
};

test('加工：先用料后产出，组件按每件用量乘计划数量，成品自己每件是 1', () => {
  deepEqual(instructionMaterials(base), [
    { direction: 'consume', goodsCode: 'PART-A', perFinished: 2, total: 20, finished: false },
    { direction: 'consume', goodsCode: 'PART-B', perFinished: 1, total: 10, finished: false },
    { direction: 'produce', goodsCode: 'KIT-1', perFinished: 1, total: 10, finished: true },
  ]);
});

test('拆解是加工反过来：用掉成品，产出组件，数量算法不变', () => {
  deepEqual(instructionMaterials({ ...base, kind: 'DISASSEMBLE', quantity: 3 }), [
    { direction: 'consume', goodsCode: 'KIT-1', perFinished: 1, total: 3, finished: true },
    { direction: 'produce', goodsCode: 'PART-A', perFinished: 2, total: 6, finished: false },
    { direction: 'produce', goodsCode: 'PART-B', perFinished: 1, total: 3, finished: false },
  ]);
});

test('换标与陌生种类不用料；读口没答组成时只剩成品那一行，不编组件', () => {
  equal(instructionMaterials({ ...base, kind: 'RELABEL' }).length, 0);
  equal(instructionMaterials({ ...base, kind: 'MYSTERY' }).length, 0);
  deepEqual(instructionMaterials({ ...base, components: undefined }), [{ direction: 'produce', goodsCode: 'KIT-1', perFinished: 1, total: 10, finished: true }]);
});

const relabel: InstructionFacts = {
  kind: 'RELABEL',
  goodsCode: 'SKU-1',
  unit: '件',
  quantity: 50,
  doneQuantity: 0,
  fromAlias: 'OLD-LABEL',
  toAlias: 'NEW-LABEL',
};

test('换标：原别名与新别名没填就是无贴标，点了批号与效期才限定范围', () => {
  deepEqual(relabelPlan(relabel), { fromAlias: 'OLD-LABEL', toAlias: 'NEW-LABEL', scope: [] });
  deepEqual(relabelPlan({ ...relabel, fromAlias: undefined, batchNumber: 'B7', expiryDate: '2027-01-31' }), {
    fromAlias: null,
    toAlias: 'NEW-LABEL',
    scope: ['批号 B7', '效期 2027-01-31'],
  });
  deepEqual(relabelPlan({ ...relabel, toAlias: '' }), { fromAlias: 'OLD-LABEL', toAlias: null, scope: [] });
  equal(relabelPlan(base), null);
});

test('一句话说这份指令要做什么，三种各说各的', () => {
  equal(instructionHeadline(base), '用组件加工出 KIT-1，计划 10 套');
  equal(instructionHeadline({ ...base, kind: 'DISASSEMBLE', unit: undefined }), '把 KIT-1 拆解成组件，计划 10');
  equal(instructionHeadline(relabel), '给 SKU-1 换标，计划 50 件：从「OLD-LABEL」换成「NEW-LABEL」');
  equal(instructionHeadline({ ...relabel, fromAlias: undefined }), '给 SKU-1 换标，计划 50 件：从「无贴标」换成「NEW-LABEL」');
  equal(instructionHeadline({ ...relabel, toAlias: undefined }), '给 SKU-1 换标，计划 50 件：从「OLD-LABEL」换成「无贴标」');
  equal(instructionHeadline({ ...base, kind: 'MYSTERY' }), 'KIT-1，计划 10 套');
});

test('少做只在已执行时有：完成数量少于计划，差多少；待执行与已作废不算少做，多做也不倒扣', () => {
  equal(instructionShortfall({ status: 'DONE', quantity: 10, doneQuantity: 7 }), 3);
  equal(instructionShortfall({ status: 'DONE', quantity: 10, doneQuantity: 10 }), 0);
  equal(instructionShortfall({ status: 'DONE', quantity: 10, doneQuantity: 12 }), 0);
  equal(instructionShortfall({ status: 'PENDING', quantity: 10, doneQuantity: 0 }), 0);
  equal(instructionShortfall({ status: 'VOID', quantity: 10, doneQuantity: 0 }), 0);
});
