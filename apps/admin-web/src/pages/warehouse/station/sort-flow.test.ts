import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { buildGuidedPayload } from '../guided-forms';
import type { SortPlanBody, WaveListBody } from '../api';
import {
  acceptingScans,
  addTote,
  applyVerdict,
  dismissWrong,
  goodsIdentified,
  orderSorted,
  putAll,
  reloadPlan,
  resend,
  scanCode,
  setAside,
  sortActions,
  sortFactValues,
  sortOutcomeLabels,
  sortProgress,
  sortScreen,
  startSort,
  targetLeft,
  type SortOrder,
  type SortState,
} from './sort-flow';
import { loadSortWaves, sortFactSpec, sortPlanOf } from './sort-plan';
import { judgeWorkFactAnswer, prepareWorkFact, type WorkFactVerdict } from './work-fact-submit';

const order = (requestId: string, more: Partial<SortOrder> = {}): SortOrder => ({
  requestId,
  documentNumber: `DOC-${requestId}`,
  ownerAccount: 'OWNER-1',
  totes: [],
  lines: [{ goodsCode: 'SKU-1', unit: '件', picked: 2, sorted: 0, open: 0 }],
  ...more,
});

const done: WorkFactVerdict = { verdict: 'done', outcome: 'SORTED', headline: '记下了。' };
const refusedWith = (outcome: string): WorkFactVerdict => ({ verdict: 'refused', outcome, headline: sortOutcomeLabels[outcome] ?? outcome });

// 两张单都要 SKU-1；第二张已经有箱 T-2。
function twoOrders(): SortState {
  return startSort('W-1', [order('OBR-1'), order('OBR-2', { totes: ['T-2'] })]);
}

// Covers: 扫到一件货，归给要这种货、还有没分的单；已经有箱的单先装，屏上说放进哪个箱；这张单还没有箱就让人扫一个空箱。
test('扫到的货先归已经有箱的单；没有箱的单要先扫一个空箱', () => {
  const first = scanCode(twoOrders(), 'SKU-1');
  equal(first.state.phase, 'tote');
  equal(first.cue, 'ok');
  deepEqual(first.state.target, { requestId: 'OBR-2', goodsCode: 'SKU-1', quantity: 1, adding: false });
  ok(first.state.feedback.text.includes('归出库单 DOC-OBR-2') && first.state.feedback.text.includes('放进箱 T-2'));
  equal(sortScreen(first.state).headline, '放进箱 T-2，扫箱确认');

  const none = scanCode(startSort('W-1', [order('OBR-1'), order('OBR-2')]), 'SKU-1');
  equal(none.state.target?.requestId, 'OBR-1');
  ok(none.state.feedback.text.includes('它还没有箱'));
  equal(sortScreen(none.state).headline, '拿一个空箱，扫箱号');
});

// Covers: 扫箱确认才提交：扫到这张单的箱，提交一件；记成之后这张单的已分数加一，回到等货，进度跟着变。
test('扫箱确认后提交一件，记成后回到等货', () => {
  const atTote = scanCode(twoOrders(), 'SKU-1').state;
  const submitted = scanCode(atTote, 'T-2');
  equal(submitted.state.phase, 'submitting');
  deepEqual(submitted.effects, [{ kind: 'submit', submission: { requestId: 'OBR-2', goodsCode: 'SKU-1', quantity: 1, tote: 'T-2' } }]);
  ok(!acceptingScans(submitted.state));
  const settled = applyVerdict(submitted.state, done);
  equal(settled.state.phase, 'unit');
  equal(settled.state.put, 1);
  equal(settled.state.orders[1].lines[0].sorted, 1);
  ok(settled.state.feedback.text.includes('1 件 SKU-1 放进了箱 T-2') && settled.state.feedback.text.includes('还有 1 件没分'));
  equal(sortProgress(settled.state), '分完 0 / 2 张，还有 3 件没分');
});

// Covers: 新箱记成之后挂到这张单名下，后面这张单的货都放它；一张单分完屏上说可以去复核打包，整个波次分完也说。
test('空箱记成后归这张单；分完一张单、分完整个波次各说一句', () => {
  let state = startSort('W-1', [order('OBR-1', { lines: [{ goodsCode: 'SKU-1', unit: '件', picked: 1, sorted: 0, open: 0 }] })]);
  state = scanCode(state, 'SKU-1').state;
  const submitted = scanCode(state, 'T-9');
  equal(submitted.effects[0].kind, 'submit');
  const settled = applyVerdict(submitted.state, done).state;
  deepEqual(settled.orders[0].totes, ['T-9']);
  ok(orderSorted(settled.orders[0]));
  ok(settled.feedback.text.includes('出库单 DOC-OBR-1 分完了') && settled.feedback.text.includes('这个波次分完了'));
  equal(sortScreen(settled).headline, '这个波次分完了');
  // 分完之后再扫到这种货：多出来的，不记
  const extra = scanCode(settled, 'SKU-1');
  equal(extra.state.phase, 'unit');
  equal(extra.cue, 'wrong');
  ok(extra.state.feedback.text.includes('都分够了'));
});

// Covers: 等箱时扫到别的单的箱、扫到不是这张单的箱、扫到商品码，都不记；这张单已有箱时要再用一个箱，先点「加一个箱」。
test('扫错箱不记；已有箱的单加箱要先点「加一个箱」', () => {
  const state = startSort('W-1', [order('OBR-1', { totes: ['T-1'] }), order('OBR-2', { totes: ['T-2'], lines: [] })]);
  const atTote = scanCode(state, 'SKU-1').state;
  equal(atTote.target?.requestId, 'OBR-1');
  const others = scanCode(atTote, 'T-2');
  equal(others.state.phase, 'tote');
  equal(others.effects.length, 0);
  ok(others.state.feedback.text.includes('箱 T-2 是出库单 DOC-OBR-2 的'));
  equal(sortScreen(others.state).headline, '这不是这张单的箱');
  const stranger = scanCode(atTote, 'T-7');
  equal(stranger.effects.length, 0);
  ok(stranger.state.feedback.text.includes('先点「加一个箱」'));
  const goods = scanCode(atTote, 'SKU-1');
  ok(goods.state.feedback.text.includes('现在要扫箱'));
  equal(dismissWrong(goods.state).state.feedback.kind, 'waiting');

  ok(sortActions(atTote).addTote);
  const adding = addTote(atTote).state;
  ok(!sortActions(adding).addTote);
  const added = scanCode(adding, 'T-7');
  deepEqual(added.effects, [{ kind: 'submit', submission: { requestId: 'OBR-1', goodsCode: 'SKU-1', quantity: 1, tote: 'T-7' } }]);
  deepEqual(applyVerdict(added.state, done).state.orders[0].totes, ['T-1', 'T-7']);
});

// Covers: 等货时扫到箱号不当成货；这个波次里没有单要的货、认不出的码都不记。
test('等货时扫到箱或没有单要的货，不记', () => {
  const state = twoOrders();
  const tote = scanCode(state, 'T-2');
  equal(tote.state.phase, 'unit');
  ok(tote.state.feedback.text.includes('现在要扫货'));
  const unknown = scanCode(state, 'BARCODE-X');
  equal(unknown.state.phase, 'resolving');
  deepEqual(unknown.effects, [{ kind: 'identify', code: 'BARCODE-X', owners: ['OWNER-1'] }]);
  const nobody = goodsIdentified(unknown.state, 'BARCODE-X', { 'OWNER-1': null });
  equal(nobody.state.phase, 'unit');
  ok(nobody.state.feedback.text.includes('没有单要这种货'));
  const failed = goodsIdentified(unknown.state, 'BARCODE-X', null, '网络没通');
  ok(failed.state.feedback.text.includes('没能认出 BARCODE-X：网络没通'));
  // 认的不是这一次扫的码：过期了，不动
  equal(goodsIdentified(unknown.state, 'OTHER', { 'OWNER-1': 'SKU-1' }).state, unknown.state);
});

// Covers: 条码按货主各认一次：同一个码在两位货主名下指向不同的货，各归各的单。
test('条码按货主认：认成哪位货主的货就归哪位货主的单', () => {
  const state = startSort('W-1', [
    order('OBR-1', { ownerAccount: 'OWNER-1', lines: [{ goodsCode: 'SKU-A', unit: '件', picked: 1, sorted: 0, open: 0 }] }),
    order('OBR-2', { ownerAccount: 'OWNER-2', lines: [{ goodsCode: 'SKU-B', unit: '件', picked: 1, sorted: 0, open: 0 }] }),
  ]);
  const asked = scanCode(state, 'EAN-1');
  deepEqual(asked.effects, [{ kind: 'identify', code: 'EAN-1', owners: ['OWNER-1', 'OWNER-2'] }]);
  const second = goodsIdentified(asked.state, 'EAN-1', { 'OWNER-1': null, 'OWNER-2': 'SKU-B' });
  deepEqual(second.state.target, { requestId: 'OBR-2', goodsCode: 'SKU-B', quantity: 1, adding: false });
  // 认成这位货主的另一种货：他的单不要它
  const other = goodsIdentified(asked.state, 'EAN-1', { 'OWNER-1': 'SKU-Z', 'OWNER-2': null });
  equal(other.state.phase, 'unit');
  ok(other.state.feedback.text.includes('没有单要这种货'));
});

// Covers: 「剩下的都放进去」一次记这张单这种货还没分的全部，再点改回一件；「这一件先放一边」不记，回到等货。
test('剩下的都放进去、这一件先放一边', () => {
  const atTote = scanCode(startSort('W-1', [order('OBR-1', { totes: ['T-1'], lines: [{ goodsCode: 'SKU-1', unit: '件', picked: 5, sorted: 2, open: 1 }] })]), 'SKU-1').state;
  equal(targetLeft(atTote), 3);
  ok(sortActions(atTote).putAll);
  const all = putAll(atTote).state;
  equal(all.target?.quantity, 3);
  equal(putAll(all).state.target?.quantity, 1);
  const submitted = scanCode(all, 'T-1');
  deepEqual(submitted.effects, [{ kind: 'submit', submission: { requestId: 'OBR-1', goodsCode: 'SKU-1', quantity: 3, tote: 'T-1' } }]);
  const settled = applyVerdict(submitted.state, done).state;
  equal(settled.orders[0].lines[0].sorted, 5);
  equal(settled.put, 3);
  // 拿回来的都分完了，还有一件没拣回来：不算分完
  ok(!orderSorted(settled.orders[0]));
  equal(sortScreen(settled).headline, '拿回来的都分完了');
  ok(settled.feedback.text.includes('还有 1 件没拣回来'));

  const aside = setAside(atTote);
  equal(aside.state.phase, 'unit');
  equal(aside.state.target, null);
  equal(aside.effects.length, 0);
});

// Covers: 后端不认箱：这一件还归这张单，换一个箱再扫（换的那一个算加给它）；内容不合、状态不符回到等货并重读；
// 没送到等「再送一次」，用同一笔。
test('后端没记成：箱不认换箱再扫，别的回到等货并重读，没送到再送一次', () => {
  const submitted = scanCode(scanCode(twoOrders(), 'SKU-1').state, 'T-2').state;
  const inUse = applyVerdict(submitted, refusedWith('TOTE_IN_USE'));
  equal(inUse.state.phase, 'tote');
  equal(inUse.state.target?.adding, true);
  equal(inUse.effects.length, 0);
  ok(inUse.state.feedback.text.includes('换一个空箱再扫'));
  equal(scanCode(inUse.state, 'T-8').effects[0].kind, 'submit');

  const stale = applyVerdict(submitted, refusedWith('CONTENT_INVALID'));
  equal(stale.state.phase, 'unit');
  equal(stale.state.target, null);
  deepEqual(stale.effects, [{ kind: 'reload' }]);
  equal(applyVerdict(submitted, refusedWith('STATUS_MISMATCH')).effects.length, 1);
  // 接入被拒没有业务答复：重读也没用，不读
  equal(applyVerdict(submitted, { verdict: 'refused', headline: '没有授予', access: 'notGranted' }).effects.length, 0);

  const unsent = applyVerdict(submitted, { verdict: 'retry', headline: '没送到' });
  equal(unsent.state.phase, 'unsent');
  ok(sortActions(unsent.state).resend && !acceptingScans(unsent.state));
  const again = resend(unsent.state);
  deepEqual(again.effects, [{ kind: 'resend' }]);
  // 重送答已有结果：算记成
  const replay = judgeWorkFactAnswer({ kind: 'outcome', status: 200, body: { outcome: 'EXISTING_RESULT' } }, sortOutcomeLabels);
  equal(applyVerdict(again.state, replay).state.orders[1].lines[0].sorted, 1);
});

// Covers: 重读回来的分拣情况只在等货时换上去；这一趟分了几件不清零。
test('重读只在等货时换上去', () => {
  const fresh = [order('OBR-9')];
  const idle = { ...twoOrders(), put: 4 };
  const reloaded = reloadPlan(idle, fresh).state;
  deepEqual(reloaded.orders, fresh);
  equal(reloaded.put, 4);
  const atTote = scanCode(twoOrders(), 'SKU-1').state;
  equal(reloadPlan(atTote, fresh).state, atTote);
});

// Covers: 一次投放装出来的报文带波次、请求、商品、件数、箱号，件数是数；装法与别的作业事实同一份。
test('投放的报文：波次、请求、商品、件数与箱号', () => {
  const values = sortFactValues({ requestId: 'OBR-1', goodsCode: 'SKU-1', quantity: 3, tote: 'T-1' }, 'W-1');
  const built = buildGuidedPayload(sortFactSpec.form, { factId: 'F-1', warehouse: 'WH-1', ...values });
  ok(built.ok);
  deepEqual(built.ok ? built.payload : null, {
    factId: 'F-1',
    warehouse: 'WH-1',
    waveId: 'W-1',
    requestId: 'OBR-1',
    goodsCode: 'SKU-1',
    quantity: 3,
    toteCode: 'T-1',
  });
  const prepared = prepareWorkFact({ spec: sortFactSpec, deviceId: 'DEV-1', warehouse: 'WH-1', sequence: 1, values });
  ok(prepared.ok && prepared.attempt.endpoint === '/warehouse-fulfillment-sort-puts');
  ok(prepared.ok && Object.keys(prepared.attempt.payload).sort().join(',') === 'factId,goodsCode,quantity,requestId,toteCode,warehouse,waveId');
});

const planAnswer = (body: SortPlanBody) => ({ kind: 'outcome', status: 200, body }) as const;

// Covers: 分拣情况读口的几种答复：读到了、不用分拣、没有这个波次、不是本仓的波次、没问成，各说各的。
test('分拣情况的答复各说各的', () => {
  const found = sortPlanOf(
    planAnswer({
      outcome: 'SORT_PLAN_FOUND',
      waveId: 'W-1',
      warehouseCode: 'WH-1',
      requests: [{ requestId: 'OBR-1', ownerAccount: 'OWNER-1', status: 'PICKED', totes: [], lines: [{ goodsCode: 'SKU-1', unit: '件', picked: 2, sorted: 1, open: 0 }] }],
    }),
    'W-1',
    'WH-1',
  );
  ok(found.kind === 'plan' && found.orders.length === 1 && found.orders[0].documentNumber === '' && found.orders[0].lines[0].sorted === 1);
  const empty = sortPlanOf(planAnswer({ outcome: 'SORT_PLAN_FOUND', waveId: 'W-1', warehouseCode: 'WH-1' }), 'W-1', 'WH-1');
  ok(empty.kind === 'plan' && empty.orders.length === 0);
  const plain = sortPlanOf(planAnswer({ outcome: 'SORT_NOT_REQUIRED', waveId: 'W-1', warehouseCode: 'WH-1' }), 'W-1', 'WH-1');
  ok(plain.kind === 'refused' && plain.reason.includes('不用分拣'));
  const missing = sortPlanOf(planAnswer({ outcome: 'WAVE_NOT_FOUND' }), 'W-1', 'WH-1');
  ok(missing.kind === 'refused' && missing.reason.includes('没有波次 W-1'));
  const elsewhere = sortPlanOf(planAnswer({ outcome: 'SORT_PLAN_FOUND', waveId: 'W-1', warehouseCode: 'WH-2' }), 'W-1', 'WH-1');
  ok(elsewhere.kind === 'refused' && elsewhere.reason.includes('仓 WH-2'));
  const down = sortPlanOf({ kind: 'transport', message: '断了' }, 'W-1', 'WH-1');
  equal(down.kind, 'failed');
});

// Covers: 选波次只问本仓合并拣货、没取消的波次的第一页；还有下一页时如实带出。
test('选波次：按本仓、合并拣货、没取消来问，只读第一页', async () => {
  const asked: unknown[] = [];
  const body: WaveListBody = {
    outcome: 'WAVES_LISTED',
    waves: [
      {
        waveId: 'W-1',
        warehouseCode: 'WH-1',
        operator: 'op',
        prefetch: false,
        status: 'PICKING',
        releasedAt: '2026-11-26T00:00:00Z',
        batchPick: true,
        requests: ['OBR-1', 'OBR-2', 'OBR-3'],
        withdrawnRequests: ['OBR-3'],
        tasks: [],
      },
    ],
    page: { next: 'cursor-2' } as WaveListBody['page'],
  };
  const answer = await loadSortWaves('WH-1', async (conditions, after) => {
    asked.push([conditions, after]);
    return { kind: 'outcome', status: 200, body };
  });
  deepEqual(asked, [[{ filters: { warehouseCode: ['WH-1'], batchPick: ['true'], status: ['RELEASED', 'PICKING', 'COMPLETED'] } }, null]]);
  ok(answer.kind === 'waves' && answer.more && answer.waves[0].requests === 2 && answer.waves[0].waveId === 'W-1');
});
