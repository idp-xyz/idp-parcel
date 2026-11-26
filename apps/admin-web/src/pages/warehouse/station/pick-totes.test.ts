import { deepEqual, equal } from 'node:assert/strict';
import { test } from 'node:test';
import type { ApiResult } from '../../catalogue-api';
import type { SettingsOverviewBody, ToteListBody, ToteRecord } from '../api';
import { applyVerdict, pickFactValues, pickScreen, scanCode, setToteMode, skipTote, startPick, pickActions } from './pick-flow';
import type { PickCell } from './pick-queue';
import { warehouseHasTotes, warehouseRequiresTotes, warehouseToteMode, type SettingsReader, type ToteReader } from './pick-totes';

const tote = (code: string, state: string): ToteRecord => ({
  warehouseCode: 'SYN-WH-01',
  toteCode: code,
  state,
  registeredBy: 'op',
  registeredAt: '2026-11-26T00:00:00Z',
});

function reader(byState: Record<string, ToteRecord[] | 'fail'>, asked: string[] = []): ToteReader {
  return async (conditions) => {
    const state = conditions.filters?.state?.[0] ?? '';
    asked.push(`${conditions.filters?.warehouseCode?.[0]}/${state}`);
    const rows = byState[state] ?? [];
    if (rows === 'fail') return { kind: 'unreachable' } as unknown as ApiResult<ToteListBody>;
    return { kind: 'outcome', status: 200, body: { outcome: 'TOTES_LISTED', totes: rows, page: { size: 50, next: null, total: rows.length } } };
  };
}

// Covers: 这个仓有空着或在用的箱才提示扫箱；只有停用的箱、没有箱、读不成都不提示。
test('这个仓有能用的周转箱才提示扫箱', async () => {
  const asked: string[] = [];
  equal(await warehouseHasTotes('SYN-WH-01', reader({ FREE: [tote('T-1', 'FREE')] }, asked)), true);
  deepEqual(asked, ['SYN-WH-01/FREE']);
  equal(await warehouseHasTotes('SYN-WH-01', reader({ IN_USE: [tote('T-1', 'IN_USE')] })), true);
  equal(await warehouseHasTotes('SYN-WH-01', reader({ RETIRED: [tote('T-1', 'RETIRED')] })), false);
  equal(await warehouseHasTotes('SYN-WH-01', reader({})), false);
  equal(await warehouseHasTotes('SYN-WH-01', reader({ FREE: 'fail' })), false);
});

const cell = (taskId: string, requestId: string, locationCode: string): PickCell => ({
  taskId,
  waveId: 'W-1',
  requestId,
  locationCode,
  goodsCode: 'SKU-1',
  unit: '件',
  stockState: 'GOOD',
  quantity: 1,
});
const A1 = cell('T-A1', 'OBR-A', 'L-1');
const B1 = cell('T-B1', 'OBR-B', 'L-2');
const A2 = cell('T-A2', 'OBR-A', 'L-3');

// Covers: 仓里没登记箱时与以前一样，扫完库位直接扫商品，报文不带箱号。
test('没开扫箱时扫完库位直接扫商品', () => {
  const started = startPick({ cells: [A1], staging: 'OUT-1' });
  const atGoods = scanCode(started, 'L-1').state;
  equal(atGoods.phase, 'goods');
  const submitted = scanCode(atGoods, 'SKU-1');
  equal(submitted.state.phase, 'submitting');
  deepEqual(submitted.effects, [{ kind: 'submit', cell: A1, quantity: 1 }]);
  deepEqual(pickFactValues({ cell: A1, quantity: 1 }, 'OUT-1'), { taskId: 'T-A1', quantity: '1', targetLocation: 'OUT-1' });
});

// Covers: 开了扫箱，一张出库单的第一格先定箱，同一张单后面的格沿用、不再问；别的单要另扫一个箱，
// 这一趟已经给了别的单的箱不收；提交的报文带箱号。
test('一张出库单的第一格扫箱，后面的格沿用；一个箱不给两张单', () => {
  let state = setToteMode(startPick({ cells: [A1, B1, A2], staging: 'OUT-1' }), true).state;
  state = scanCode(state, 'L-1').state;
  equal(state.phase, 'tote');
  equal(pickScreen(state).headline, '扫这张单的周转箱');
  equal(pickActions(state).skipTote, true);
  // 库位码、商品码都不是箱
  equal(scanCode(state, 'L-1').state.feedback.kind, 'wrong');
  equal(scanCode(state, 'SKU-1').state.feedback.kind, 'wrong');
  equal(scanCode(state, 'OUT-1').state.phase, 'tote');
  state = scanCode(state, 'TOTE-1').state;
  equal(state.phase, 'goods');
  equal(state.totes['OBR-A'], 'TOTE-1');
  const first = scanCode(state, 'SKU-1');
  deepEqual(first.effects, [{ kind: 'submit', cell: A1, quantity: 1, tote: 'TOTE-1' }]);
  deepEqual(pickFactValues({ cell: A1, quantity: 1, tote: 'TOTE-1' }, 'OUT-1'), {
    taskId: 'T-A1',
    quantity: '1',
    targetLocation: 'OUT-1',
    toteCode: 'TOTE-1',
  });
  state = applyVerdict(first.state, { verdict: 'done', outcome: 'PICKED', headline: '已记下' }).state;

  // 第二格是另一张单：要另扫一个箱，头一张单的箱不收
  state = scanCode(state, 'L-2').state;
  equal(state.phase, 'tote');
  const reused = scanCode(state, 'TOTE-1').state;
  equal(reused.phase, 'tote');
  equal(reused.feedback.kind, 'wrong');
  state = scanCode(state, 'TOTE-2').state;
  state = applyVerdict(scanCode(state, 'SKU-1').state, { verdict: 'done', outcome: 'PICKED', headline: '已记下' }).state;

  // 第三格又是头一张单：不再问箱，直接扫商品，屏上写着放进哪个箱
  state = scanCode(state, 'L-3').state;
  equal(state.phase, 'goods');
  equal(pickScreen(state).counter, '已扫 0 / 1，放进箱 TOTE-1');
  deepEqual(scanCode(state, 'SKU-1').effects, [{ kind: 'submit', cell: A2, quantity: 1, tote: 'TOTE-1' }]);
});

// Covers: 「不用箱」只在等箱那一步能点；点了之后这张单这一趟不再问箱，报文不带箱号。
test('不用箱：这张单这一趟不再问', () => {
  let state = setToteMode(startPick({ cells: [A1, A2], staging: 'OUT-1' }), true).state;
  deepEqual(skipTote(state).state, state);
  state = skipTote(scanCode(state, 'L-1').state).state;
  equal(state.phase, 'goods');
  equal(state.totes['OBR-A'], null);
  const first = scanCode(state, 'SKU-1');
  deepEqual(first.effects, [{ kind: 'submit', cell: A1, quantity: 1 }]);
  state = applyVerdict(first.state, { verdict: 'done', outcome: 'PICKED', headline: '已记下' }).state;
  equal(scanCode(state, 'L-3').state.phase, 'goods');
});

// Covers: 后端不认这个箱（绑着别的单、没登记、已停用）时这一笔没记，这张单定下的箱忘掉；这一格排到队尾，回头再来重新扫箱。
test('后端不认的箱要忘掉，回头重新扫', () => {
  for (const outcome of ['TOTE_IN_USE', 'TOTE_NOT_FOUND', 'TOTE_NOT_ACCEPTING']) {
    let state = setToteMode(startPick({ cells: [A1, B1], staging: 'OUT-1' }), true).state;
    state = scanCode(scanCode(state, 'L-1').state, 'TOTE-1').state;
    state = applyVerdict(scanCode(state, 'SKU-1').state, { verdict: 'refused', outcome, headline: '没有记' }).state;
    equal(state.phase, 'refused', outcome);
    equal(state.gone, false, outcome);
    equal(state.totes['OBR-A'], undefined, outcome);
  }
  // 别的拒绝不动定下的箱
  let state = setToteMode(startPick({ cells: [A1, B1], staging: 'OUT-1' }), true).state;
  state = scanCode(scanCode(state, 'L-1').state, 'TOTE-1').state;
  state = applyVerdict(scanCode(state, 'SKU-1').state, { verdict: 'refused', outcome: 'CONTENT_INVALID', headline: '没有记' }).state;
  equal(state.totes['OBR-A'], 'TOTE-1');
});

const settings = (toteRequirements: SettingsOverviewBody['toteRequirements'] | 'fail'): SettingsReader => async () =>
  toteRequirements === 'fail'
    ? ({ kind: 'unreachable' } as unknown as ApiResult<SettingsOverviewBody>)
    : { kind: 'outcome', status: 200, body: { outcome: 'SETTINGS_OVERVIEW_LOADED', toteRequirements } as SettingsOverviewBody };

const requirement = (warehouseCode: string, required: boolean) => ({
  warehouseCode,
  revision: 1,
  required,
  operator: 'op',
  registeredAt: '2026-11-26T00:00:00Z',
});

// Covers: 租户给这个仓登记了必须用箱才是要求；登记成不要求、登记的是别的仓、没登记、读不成都不是。要求用箱时一定提示扫箱，
// 哪怕这个仓一个箱都没登记。
test('这个仓要不要求用箱取自租户登记的规则', async () => {
  equal(await warehouseRequiresTotes('SYN-WH-01', settings([requirement('SYN-WH-01', true)])), true);
  equal(await warehouseRequiresTotes('SYN-WH-01', settings([requirement('SYN-WH-01', false)])), false);
  equal(await warehouseRequiresTotes('SYN-WH-01', settings([requirement('SYN-WH-02', true)])), false);
  equal(await warehouseRequiresTotes('SYN-WH-01', settings([])), false);
  equal(await warehouseRequiresTotes('SYN-WH-01', settings(undefined)), false);
  equal(await warehouseRequiresTotes('SYN-WH-01', settings('fail')), false);
  deepEqual(await warehouseToteMode('SYN-WH-01', reader({}), settings([requirement('SYN-WH-01', true)])), { on: true, required: true });
  deepEqual(await warehouseToteMode('SYN-WH-01', reader({ FREE: [tote('T-1', 'FREE')] }), settings([])), { on: true, required: false });
  deepEqual(await warehouseToteMode('SYN-WH-01', reader({}), settings([])), { on: false, required: false });
});

// Covers: 要求用箱时不能点「不用箱」，屏上说明这个仓必须用箱；这一趟此前点过「不用箱」的出库单要重新问。
test('必须用箱：不能点不用箱，点过不用箱的单重新问', () => {
  let state = setToteMode(startPick({ cells: [A1, A2], staging: 'OUT-1' }), true, true).state;
  state = scanCode(state, 'L-1').state;
  equal(state.phase, 'tote');
  equal(pickActions(state).skipTote, false);
  deepEqual(skipTote(state).state, state);
  equal(pickScreen(state).counter.includes('必须用周转箱'), true);

  // 开工时还不要求：头一张单点了不用箱；规则读回来是要求，那个决定作废
  let declined = setToteMode(startPick({ cells: [A1, A2], staging: 'OUT-1' }), true).state;
  declined = skipTote(scanCode(declined, 'L-1').state).state;
  equal(declined.totes['OBR-A'], null);
  const required = setToteMode(declined, true, true).state;
  equal(required.totes['OBR-A'], undefined);
  equal(required.toteRequired, true);
});

// Covers: 工位以为不要求、后端答必须用箱（规则是这一趟开工之后才登记的）：这一笔没记，工位从此按必须用箱来，这张单重新问箱。
test('后端答必须用箱之后工位改成必须用箱', () => {
  let state = startPick({ cells: [A1, B1], staging: 'OUT-1' });
  state = scanCode(state, 'L-1').state;
  equal(state.phase, 'goods');
  state = applyVerdict(scanCode(state, 'SKU-1').state, { verdict: 'refused', outcome: 'TOTE_REQUIRED', headline: '没有记' }).state;
  equal(state.phase, 'refused');
  equal(state.gone, false);
  equal(state.toteOn, true);
  equal(state.toteRequired, true);
});

// Covers: 名册读回来是没有箱、而人正停在等箱那一步时，直接去扫商品。
test('读回来没有箱时不卡在等箱', () => {
  const waiting = scanCode(setToteMode(startPick({ cells: [A1], staging: 'OUT-1' }), true).state, 'L-1').state;
  equal(waiting.phase, 'tote');
  equal(setToteMode(waiting, false).state.phase, 'goods');
});
