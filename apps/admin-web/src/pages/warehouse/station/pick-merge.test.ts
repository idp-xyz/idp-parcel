import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import type { WaveRecord, WaveTaskRecord } from '../api';
import {
  applyVerdict,
  nextCell,
  pickFactValues,
  pickProgress,
  pickScreen,
  scanCode,
  setToteMode,
  shortPickLines,
  shortPickPlan,
  startPick,
  submitShortPick,
  type PickState,
} from './pick-flow';
import { cellsOfWaves, type PickCell } from './pick-queue';

const task = (taskId: string, requestId: string, locationCode: string, more: Partial<WaveTaskRecord> = {}): WaveTaskRecord => ({
  taskId,
  requestId,
  ownerAccount: 'OWNER-1',
  locationCode,
  goodsCode: 'SKU-1',
  unit: '件',
  stockState: 'GOOD',
  quantity: 1,
  picked: 0,
  status: 'OPEN',
  ...more,
});

const wave = (waveId: string, singleUnit: boolean, tasks: WaveTaskRecord[]): WaveRecord => ({
  waveId,
  warehouseCode: 'WH-1',
  operator: 'op',
  prefetch: false,
  status: 'RELEASED',
  releasedAt: '2026-11-26T00:00:00Z',
  singleUnit,
  requests: tasks.map((item) => item.requestId),
  withdrawnRequests: [],
  tasks,
});

const scope = { warehouse: 'WH-1', wave: null };

// Covers: 单品单件波次里同一库位、同一位货主的同一种货合成一格，应拣数量是任务条数；别的库位、别的货主、别的商品各是各的格；
// 不是单品单件的波次不合。
test('单品单件波次里同库位同货主同商品的任务合成一格', () => {
  const cells = cellsOfWaves(
    [
      wave('W-1', true, [
        task('T-3', 'OBR-3', 'L-1'),
        task('T-1', 'OBR-1', 'L-1'),
        task('T-2', 'OBR-2', 'L-1'),
        task('T-4', 'OBR-4', 'L-2'),
        task('T-5', 'OBR-5', 'L-1', { ownerAccount: 'OWNER-2' }),
        task('T-6', 'OBR-6', 'L-1', { goodsCode: 'SKU-2' }),
        task('T-7', 'OBR-7', 'L-1', { status: 'DONE', picked: 1 }),
      ]),
      wave('W-2', false, [task('T-8', 'OBR-8', 'L-1'), task('T-9', 'OBR-9', 'L-1')]),
    ],
    scope,
  );
  const merged = cells.find((cell) => cell.members !== undefined);
  equal(merged?.quantity, 3);
  equal(merged?.taskId, 'T-1');
  deepEqual(merged?.members?.map((member) => member.taskId), ['T-1', 'T-2', 'T-3']);
  equal(cells.filter((cell) => cell.members !== undefined).length, 1);
  // 别的货主、别的商品、别的库位、不是单品单件的波次里的两条：各一格
  equal(cells.length, 6);
  equal(cells.filter((cell) => cell.singleUnit === true).length, 4);
});

// Covers: 读口没带货主时不合——分不清是不是同一位货主的货。
test('读口没带货主时不合', () => {
  const cells = cellsOfWaves(
    [wave('W-1', true, [task('T-1', 'OBR-1', 'L-1', { ownerAccount: undefined }), task('T-2', 'OBR-2', 'L-1', { ownerAccount: undefined })])],
    scope,
  );
  equal(cells.length, 2);
  equal(cells.every((cell) => cell.members === undefined), true);
});

function mergedCell(count: number): PickCell {
  const tasks = Array.from({ length: count }, (_, index) => task(`T-${index + 1}`, `OBR-${index + 1}`, 'L-1'));
  return cellsOfWaves([wave('W-1', true, tasks)], scope)[0];
}

const done = { verdict: 'done' as const, outcome: 'PICKED', headline: '已记下' };

function atGoods(cells: PickCell[]): PickState {
  return scanCode(startPick({ cells, staging: 'OUT-1' }), 'L-1').state;
}

// Covers: 合成的一格扫满之后一条任务一条任务地交，每条一件、各用自己的任务号，不带箱号；都记成了这一格才出队。
test('合成的一格扫满后逐条提交，都记成才出队', () => {
  const cell = mergedCell(3);
  let state = atGoods([cell]);
  ok(pickScreen(startPick({ cells: [cell], staging: 'OUT-1' })).counter.includes('3 张单品单件的出库单'));
  state = scanCode(scanCode(state, 'SKU-1').state, 'SKU-1').state;
  equal(state.phase, 'goods');
  const full = scanCode(state, 'SKU-1');
  equal(full.state.phase, 'submitting');
  equal(full.effects.length, 1);
  const first = full.effects[0];
  ok(first.kind === 'submit' && first.cell.taskId === 'T-1' && first.quantity === 1 && first.cell.members === undefined);
  if (first.kind === 'submit') {
    deepEqual(pickFactValues({ cell: first.cell, quantity: first.quantity }, 'OUT-1'), { taskId: 'T-1', quantity: '1', targetLocation: 'OUT-1' });
  }
  const second = applyVerdict(full.state, done);
  equal(second.state.phase, 'submitting');
  ok(second.effects[0].kind === 'submit' && second.effects[0].cell.taskId === 'T-2');
  const third = applyVerdict(second.state, done);
  ok(third.effects[0].kind === 'submit' && third.effects[0].cell.taskId === 'T-3');
  const finished = applyVerdict(third.state, done);
  equal(finished.state.phase, 'finished');
  equal(finished.state.batch, null);
  equal(finished.state.done, 1);
  equal(finished.effects.length, 0);
  ok(finished.state.feedback.text.includes('记给了 3 张出库单'));
});

// Covers: 没拿够时按拿到的件数记给头几张出库单；没拿到的那几条任务不记短拣，仍合成一格排到队尾。
test('合成的一格没拿够：拿到几件记几张，剩下的留在队尾', () => {
  const cell = mergedCell(3);
  let state = scanCode(atGoods([cell]), 'SKU-1').state;
  const plan = shortPickPlan(state);
  equal(plan?.quantity, 1);
  equal(plan?.short, 2);
  ok(plan !== null && shortPickLines(plan).some((line) => line.includes('原样留着')));
  const submitted = submitShortPick(state);
  equal(submitted.state.phase, 'submitting');
  ok(submitted.effects[0].kind === 'submit' && submitted.effects[0].cell.taskId === 'T-1');
  state = applyVerdict(submitted.state, done).state;
  equal(state.phase, 'location');
  equal(state.done, 1);
  equal(state.queue.length, 1);
  equal(state.queue[0].quantity, 2);
  deepEqual(state.queue[0].members?.map((member) => member.taskId), ['T-2', 'T-3']);
  equal(pickProgress(state), '第 2 格，共 2 格');
});

// Covers: 交到一半后端没成：已记成的几张是真的记下了，屏上说清；「下一格」去掉记成的，余下的仍合成一格排到队尾。
// 没成的那一条后端已不能拣时也去掉。
test('合成的一格交到一半没成', () => {
  const cell = mergedCell(3);
  let state = atGoods([cell, { ...task('T-9', 'OBR-9', 'L-9'), waveId: 'W-1' } as unknown as PickCell]);
  state = scanCode(scanCode(scanCode(state, 'SKU-1').state, 'SKU-1').state, 'SKU-1').state;
  state = applyVerdict(state, done).state;
  const refused = applyVerdict(state, { verdict: 'refused', outcome: 'CONTENT_INVALID', headline: '没有记。' }).state;
  equal(refused.phase, 'refused');
  ok(refused.feedback.text.includes('已经记下 1 张出库单'));
  const moved = nextCell(refused).state;
  equal(moved.done, 1);
  equal(moved.batch, null);
  deepEqual(moved.queue[moved.queue.length - 1].members?.map((member) => member.taskId), ['T-2', 'T-3']);

  const gone = applyVerdict(state, { verdict: 'refused', outcome: 'CONTENT_CONFLICT', headline: '已经拣过。' }).state;
  const dropped = nextCell(gone).state;
  deepEqual(dropped.queue[dropped.queue.length - 1].members?.map((member) => member.taskId), ['T-3']);
});

// Covers: 单品单件波次的格不问周转箱，哪怕这个仓登记了箱：它的货合在一起拿。
test('单品单件波次的格不问周转箱', () => {
  const cell = mergedCell(2);
  const state = scanCode(setToteMode(startPick({ cells: [cell], staging: 'OUT-1' }), true).state, 'L-1').state;
  equal(state.phase, 'goods');
});
