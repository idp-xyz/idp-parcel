import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { OutboundRequestRecord } from '../api';
import {
  acceptingScans,
  addParcel,
  applyVerdict,
  backToForm,
  changeCounts,
  checkOutcomeLabels,
  chooseMaterial,
  countedTotal,
  dismissWrong,
  editParcel,
  expectedTotal,
  goodsIdentified,
  judgeCounts,
  leaveOrder,
  rematchAfterRefusal,
  mismatchLines,
  mismatchPlan,
  mismatchText,
  openOrder,
  orderFound,
  orderOpened,
  packActions,
  packagingIdentified,
  packHeadline,
  packCommissionWords,
  packOutcomeLabels,
  packPacked,
  packProgress,
  packResendable,
  packScreen,
  referenceGoods,
  removeParcel,
  resend,
  scanCode,
  scanPlaceholder,
  scanTarget,
  startPack,
  submitMismatch,
  submitPack,
  type PackPhase,
  type PackState,
  type PackStep,
} from './pack-flow';
import type { OrderLookup, OrderOpened, PackGoods, PackOrder, PackQueue } from './pack-order';
import type { PackagingMaterial } from './pack-parcels';
import { pickStationHash } from './stations';
import { judgeWorkFactAnswer, type WorkFactVerdict } from './work-fact-submit';

const WH = 'SYN-WH-01';
const G1: PackGoods = { goodsCode: 'SYN-GOODS-01', unit: '件', quantity: 3 };
const G2: PackGoods = { goodsCode: 'SYN-GOODS-02', unit: '件', quantity: 2 };

function order(over: Partial<PackOrder> = {}): PackOrder {
  return {
    requestId: 'OBR-1',
    number: 'SYN-OUT-001',
    ownerAccount: 'SYN-OWNER-01',
    warehouse: WH,
    dispatch: 'PRODUCT_NETWORK',
    status: 'PICKED',
    goods: [G1, G2],
    lastMismatch: null,
    ...over,
  };
}

function rowOf(o: PackOrder, over: Partial<OutboundRequestRecord> = {}): OutboundRequestRecord {
  return {
    requestId: o.requestId,
    documentNumber: o.number,
    ownerAccount: o.ownerAccount,
    warehouseCode: o.warehouse,
    origin: 'OWNER_API',
    requestKey: 'K-1',
    dispatch: o.dispatch,
    recipient: '收件人',
    address: '某地',
    status: o.status,
    platformStocking: false,
    receivedAt: '2026-11-25T08:00:00Z',
    lines: [],
    reservedQuantity: 0,
    wantedQuantity: 0,
    ...over,
  };
}

const opened = (o: PackOrder, openTasks = false): OrderOpened => ({ kind: 'order', order: o, openTasks });

const BOX: PackagingMaterial = {
  code: 'SYN-BOX-S',
  name: '小号纸箱',
  length: '30',
  width: '20',
  height: '15',
  dimensionUnit: 'CM',
  weightUnit: 'KG',
  deactivated: false,
};
/** 没登记外廓的包材（气泡袋这类）：尺寸要人填。 */
const BAG: PackagingMaterial = { ...BOX, code: 'SYN-BAG-1', name: '气泡袋', length: '', width: '', height: '', dimensionUnit: '' };

const QUEUE: PackQueue = { checkable: { rows: [], total: 4 }, packable: { rows: [], total: 2 } };

const checkVerdict = (outcome: string): WorkFactVerdict => judgeWorkFactAnswer({ kind: 'outcome', status: 201, body: { outcome } }, checkOutcomeLabels);
const packVerdict = (outcome: string): WorkFactVerdict => judgeWorkFactAnswer({ kind: 'outcome', status: 201, body: { outcome } }, packOutcomeLabels);

function unchanged(before: PackState, step: PackStep) {
  deepEqual(step.state, before);
  deepEqual(step.effects, []);
  equal(step.cue, undefined);
}

const idle = (): PackState => startPack(WH, null).state;
const finding = (code = 'SYN-OUT-001'): PackState => scanCode(idle(), code).state;

/** 扫出库单号 → 找到 → 读明细：停在逐件扫（已拣货）或打包（已复核）。 */
function atOrder(o: PackOrder): PackState {
  const code = o.number;
  const found = orderFound(finding(code), code, { kind: 'found', row: rowOf(o) });
  return orderOpened(found.state, o.requestId, opened(o)).state;
}

const checking = (o = order()): PackState => atOrder(o);
const packing = (o = order({ status: 'CHECKED' })): PackState => atOrder(o);

const scanned = (state: PackState, codes: string[]): PackState => codes.reduce((current, code) => scanCode(current, code).state, state);
const ALL = ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-02', 'SYN-GOODS-02'];

/** 两个包裹都选了包材、录了重量：包裹 1 装 G1×3，包裹 2 装 G2×2，随时能提交。 */
function readyToPack(): PackState {
  let state = addParcel(packing()).state;
  for (const parcel of state.parcels) {
    state = chooseMaterial(state, parcel.key, BOX).state;
    state = editParcel(state, parcel.key, { weight: '1.25', weightUnit: 'KG' }).state;
  }
  return state;
}

// —— 开工 ——

test('开工：地址里没带出库单就等扫出库单号；带了就直接去读那张单', () => {
  const none = startPack(WH, null);
  equal(none.state.phase, 'waiting');
  equal(none.state.warehouse, WH);
  equal(none.state.order, null);
  deepEqual(none.effects, []);
  equal(none.cue, undefined);

  const direct = startPack(WH, 'OBR-1');
  equal(direct.state.phase, 'opening');
  equal(direct.state.pending, 'OBR-1');
  deepEqual(direct.effects, [{ kind: 'open-order', requestId: 'OBR-1' }]);
});

test('地址里带的请求号读到的是已拣货的单：进逐件扫；读到的是已复核的单：直接打包', () => {
  const toCheck = orderOpened(startPack(WH, 'OBR-1').state, 'OBR-1', opened(order()));
  equal(toCheck.state.phase, 'checking');
  const toPack = orderOpened(startPack(WH, 'OBR-1').state, 'OBR-1', opened(order({ status: 'CHECKED' })));
  equal(toPack.state.phase, 'packing');
});

// —— 等扫出库单号 ——

test('等出库单号：扫到一个码就去找是哪张单，进找单中，什么都没记，也不响', () => {
  const step = scanCode(idle(), 'SYN-OUT-001');
  equal(step.state.phase, 'finding');
  equal(step.state.pending, 'SYN-OUT-001');
  equal(step.state.feedback.kind, 'waiting');
  equal(step.state.feedback.scanned, 'SYN-OUT-001');
  deepEqual(step.effects, [{ kind: 'find-order', code: 'SYN-OUT-001' }]);
  equal(step.cue, undefined);
});

test('等出库单号：枪夹带的控制字符与首尾空白不带进去找；空码不算', () => {
  deepEqual(scanCode(idle(), '\u0002 SYN-OUT-001\r\n').effects, [{ kind: 'find-order', code: 'SYN-OUT-001' }]);
  unchanged(idle(), scanCode(idle(), '\r\n'));
  unchanged(idle(), scanCode(idle(), '   '));
});

test('找单中、读明细中不收扫码，也不收点队列', () => {
  const waiting = finding();
  unchanged(waiting, scanCode(waiting, 'SYN-OUT-002'));
  unchanged(waiting, openOrder(waiting, 'OBR-2'));
  const reading = startPack(WH, 'OBR-1').state;
  unchanged(reading, scanCode(reading, 'SYN-OUT-002'));
});

test('点队列里的一张单：只在等扫出库单号时收，直接去读它的明细', () => {
  const step = openOrder(idle(), 'OBR-7');
  equal(step.state.phase, 'opening');
  equal(step.state.pending, 'OBR-7');
  deepEqual(step.effects, [{ kind: 'open-order', requestId: 'OBR-7' }]);
  const busy = checking();
  unchanged(busy, openOrder(busy, 'OBR-7'));
});

test('找到了已拣货或已复核的单：去读明细，按请求号读（不是扫到的单据号）', () => {
  for (const status of ['PICKED', 'CHECKED']) {
    const o = order({ status });
    const step = orderFound(finding(o.number), o.number, { kind: 'found', row: rowOf(o) });
    equal(step.state.phase, 'opening', status);
    equal(step.state.pending, 'OBR-1', status);
    deepEqual(step.effects, [{ kind: 'open-order', requestId: 'OBR-1' }], status);
    equal(step.cue, undefined);
  }
});

test('找到的单现在不能在这里做：回到等扫、红，各说各的、各指各的去处，什么都没记', () => {
  const statuses = ['RESERVED', 'PARTIALLY_RESERVED', 'PENDING_POLICY', 'ALLOCATED', 'PACKED', 'SHIPPED', 'CANCELLED', 'PARTIALLY_CANCELLED', 'AWAITING_RETURN', 'REJECTED', 'SPLIT', 'HANDED_OFF', 'FORWARDED'];
  const texts = new Set<string>();
  for (const status of statuses) {
    const o = order({ status });
    const step = orderFound(finding(o.number), o.number, { kind: 'found', row: rowOf(o) });
    equal(step.state.phase, 'waiting', status);
    equal(step.state.pending, null, status);
    equal(step.state.feedback.kind, 'wrong', status);
    ok(step.state.feedback.text.includes('SYN-OUT-001') && step.state.feedback.text.includes('没有记任何东西'), status);
    ok(step.state.feedback.goto !== undefined, `${status} 要指个去处`);
    equal(step.state.feedback.scanned, 'SYN-OUT-001');
    deepEqual(step.effects, []);
    equal(step.cue, 'wrong');
    texts.add(step.state.feedback.text);
  }
  equal(texts.size, statuses.length - 1, '只有代理仓的两个状态共用一句');
});

test('找到的单是别的仓的：说它属于哪个仓、这台设备绑的是哪个仓', () => {
  const o = order({ warehouse: 'SYN-WH-02' });
  const step = orderFound(finding(o.number), o.number, { kind: 'found', row: rowOf(o) });
  equal(step.state.phase, 'waiting');
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('SYN-WH-02') && step.state.feedback.text.includes(WH));
  equal(step.cue, 'wrong');
});

test('找单的另三种结果：在别的仓、真的没有、没核成，各说各的；没核成不说成没有', () => {
  const elsewhere = orderFound(finding(), 'SYN-OUT-001', { kind: 'elsewhere', warehouse: 'SYN-WH-02', number: 'SYN-OUT-001' });
  equal(elsewhere.state.feedback.kind, 'wrong');
  ok(elsewhere.state.feedback.text.includes('属于仓 SYN-WH-02') && elsewhere.state.feedback.text.includes('绑的是仓 SYN-WH-01'));
  equal(elsewhere.cue, 'wrong');
  equal(packScreen(elsewhere.state, QUEUE).headline, '这张单在别的仓');

  const absent = orderFound(finding(), 'SYN-OUT-001', { kind: 'absent' });
  equal(absent.state.phase, 'waiting');
  equal(absent.state.feedback.kind, 'wrong');
  ok(absent.state.feedback.text.includes('没有找到出库单 SYN-OUT-001') && absent.state.feedback.text.includes('没有记任何东西'));
  equal(packScreen(absent.state, QUEUE).headline, '没有这张单', '只有真问到了没有，才说没有这张单');

  const unverified = orderFound(finding(), 'SYN-OUT-001', { kind: 'unverified', reason: '读出库单没成：网络没通（断了）' });
  equal(unverified.state.phase, 'waiting');
  equal(unverified.state.feedback.kind, 'unsent');
  ok(unverified.state.feedback.text.includes('没能核对出库单 SYN-OUT-001') && unverified.state.feedback.text.includes('网络没通'));
  ok(!unverified.state.feedback.text.includes('没有找到'), '没核成不说成没有');
  equal(unverified.cue, 'wrong');
});

test('找单的答复过期了（码对不上、不在找单中）：不动', () => {
  const waiting = finding('SYN-OUT-001');
  const lookup: OrderLookup = { kind: 'absent' };
  unchanged(waiting, orderFound(waiting, 'SYN-OUT-999', lookup));
  unchanged(idle(), orderFound(idle(), 'SYN-OUT-001', lookup));
});

test('等出库单号的屏：最大的字「扫出库单号」，小字是本仓等复核与等打包的张数；扫错时变成「扫错了」', () => {
  const screen = packScreen(idle(), QUEUE);
  equal(screen.label, '下一步');
  equal(screen.headline, '扫出库单号');
  equal(screen.counter, '已拣完、等复核的出库单 4 张；已复核、等打包的 2 张');
  equal(packScreen(idle(), null).counter, '正在读本仓等复核与等打包的出库单…');

  const notHere = orderFound(finding(), 'SYN-OUT-001', { kind: 'found', row: rowOf(order({ status: 'SHIPPED' })) }).state;
  equal(packScreen(notHere, QUEUE).label, '扫错了');
  equal(packScreen(notHere, QUEUE).headline, '这张单现在不能在这里做');
  equal(packProgress(idle()), '');
  // 没核成不是「没有」：不变红，最大的字还是扫出库单号
  const unverified = orderFound(finding(), 'SYN-OUT-001', { kind: 'unverified', reason: '读出库单没成：网络没通（断了）' }).state;
  equal(packScreen(unverified, QUEUE).headline, '扫出库单号');
  equal(packScreen(unverified, QUEUE).label, '下一步');
});

// —— 读明细 ——

test('读到已拣货的单：开始逐件扫，数从 0 起，提示扫齐了自动提交复核', () => {
  const step = orderOpened(startPack(WH, 'OBR-1').state, 'OBR-1', opened(order()));
  equal(step.state.phase, 'checking');
  equal(step.state.order?.number, 'SYN-OUT-001');
  deepEqual(step.state.counted, {});
  equal(step.state.pending, null);
  ok(step.state.feedback.text.includes('逐件扫') && step.state.feedback.text.includes('自动提交复核'));
  equal(expectedTotal(order()), 5);
  equal(countedTotal(step.state), 0);
  const screen = packScreen(step.state, QUEUE);
  equal(screen.headline, '逐件扫，还差 5 件');
  equal(screen.counter, '这张单已扫 0 / 5');
  equal(packProgress(step.state), '第 1 步，共 2 步：复核');
});

test('读到的已拣货单以前复核不符过：屏上说明，免得人以为是第一次', () => {
  const o = order({ lastMismatch: { times: 2, observed: 4 } });
  const step = orderOpened(startPack(WH, 'OBR-1').state, 'OBR-1', opened(o));
  ok(step.state.feedback.text.includes('复核过 2 次') && step.state.feedback.text.includes('最近一次点了 4 件'));
});

test('读到已复核的单：直接打包，先备好第一个包裹（第一种货、已拣件数），包材重量一概不预填', () => {
  const state = packing();
  equal(state.phase, 'packing');
  equal(state.parcels.length, 1);
  const parcel = state.parcels[0];
  equal(parcel.goodsCode, 'SYN-GOODS-01');
  equal(parcel.quantity, '3');
  deepEqual([parcel.packaging, parcel.weight, parcel.weightUnit, parcel.length, parcel.width, parcel.height, parcel.dimensionUnit], ['', '', '', '', '', '', '']);
  ok(state.feedback.text.includes('已经复核过了，直接打包'));
  equal(packProgress(state), '第 2 步，共 2 步：打包');
});

test('读到的单现在不能做：还有任务没拣完、一件都没拣到、状态已变，回到等扫并说清', () => {
  const reading = startPack(WH, 'OBR-1').state;
  const openTasks = orderOpened(reading, 'OBR-1', opened(order(), true));
  equal(openTasks.state.phase, 'waiting');
  ok(openTasks.state.feedback.text.includes('没拣完') && openTasks.state.feedback.goto?.hash === pickStationHash());
  equal(openTasks.cue, 'wrong');

  const empty = orderOpened(reading, 'OBR-1', opened(order({ goods: [] })));
  ok(empty.state.feedback.text.includes('没有拣出任何货'));

  const shipped = orderOpened(reading, 'OBR-1', opened(order({ status: 'SHIPPED' })));
  equal(shipped.state.phase, 'waiting');
  ok(shipped.state.feedback.text.includes('已经出库了') && shipped.state.feedback.text.includes('没有记任何东西'));

  const packed = orderOpened(reading, 'OBR-1', opened(order({ status: 'PACKED' })));
  equal(packed.state.feedback.goto?.hash, '#/station/dispatch?request=OBR-1');
});

test('读明细：没有这张单与没读成是两回事，没读成不说成没有', () => {
  const reading = startPack(WH, 'OBR-9').state;
  const absent = orderOpened(reading, 'OBR-9', { kind: 'absent' });
  equal(absent.state.phase, 'waiting');
  equal(absent.state.feedback.kind, 'wrong');
  ok(absent.state.feedback.text.includes('没有找到出库单 OBR-9'));
  equal(packScreen(absent.state, QUEUE).headline, '没有这张单');

  const failed = orderOpened(reading, 'OBR-9', { kind: 'failed', reason: '网络没通（断了）' });
  equal(failed.state.phase, 'waiting');
  equal(failed.state.feedback.kind, 'unsent');
  ok(failed.state.feedback.text.includes('没能读出库单 OBR-9') && failed.state.feedback.text.includes('网络没通'));
  ok(!failed.state.feedback.text.includes('没有找到'));
});

test('读明细的答复过期了：不动', () => {
  const reading = startPack(WH, 'OBR-1').state;
  unchanged(reading, orderOpened(reading, 'OBR-2', opened(order())));
  unchanged(idle(), orderOpened(idle(), 'OBR-1', opened(order())));
});

// —— 复核：逐件扫 ——

test('扫到单里的货：加一件，扫对的声音，屏上写已扫几比几', () => {
  const step = scanCode(checking(), 'SYN-GOODS-01');
  equal(step.state.phase, 'checking');
  equal(countedTotal(step.state), 1);
  equal(step.state.feedback.kind, 'ok');
  equal(step.state.feedback.text, '对。SYN-GOODS-01 已扫 1 / 3。');
  equal(step.state.feedback.scanned, 'SYN-GOODS-01');
  deepEqual(step.effects, []);
  equal(step.cue, 'ok');
  const screen = packScreen(step.state, QUEUE);
  equal(screen.headline, '逐件扫，还差 4 件');
  equal(screen.counter, '这张单已扫 1 / 5');
});

test('逐件扫：货的先后不论；扫齐的那一下自动提交复核，提交的是实点总数，不再收扫码', () => {
  const almost = scanned(checking(), ['SYN-GOODS-02', 'SYN-GOODS-01', 'SYN-GOODS-02', 'SYN-GOODS-01']);
  equal(almost.phase, 'checking');
  const step = scanCode(almost, 'SYN-GOODS-01');
  equal(step.state.phase, 'check-submitting');
  equal(step.cue, 'ok');
  deepEqual(step.effects, [{ kind: 'submit-check', order: order(), observed: 5 }]);
  deepEqual(step.state.submission, { step: 'check', order: order(), observed: 5 });
  ok(step.state.feedback.text.includes('扫齐了'));
  equal(packScreen(step.state, QUEUE).headline, '正在提交复核…');
  equal(acceptingScans(step.state), false);
  unchanged(step.state, scanCode(step.state, 'SYN-GOODS-01'));
});

test('扫多了：这种货已经扫够再扫，整块红、不记数、不提交', () => {
  const full = scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01']);
  const step = scanCode(full, 'SYN-GOODS-01');
  equal(step.state.phase, 'checking');
  equal(countedTotal(step.state), 3);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('已经扫够了（这张单该有 3 件）') && step.state.feedback.text.includes('没有记数'));
  deepEqual(step.effects, []);
  equal(step.cue, 'wrong');
  const screen = packScreen(step.state, QUEUE);
  equal(screen.label, '扫错了');
  equal(screen.headline, '这件不能记数');
  equal(screen.counter, '这张单已扫 3 / 5');
});

test('扫错之后再扫对的：红清掉，接着记数', () => {
  const wrong = scanCode(scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01']), 'SYN-GOODS-01').state;
  const step = scanCode(wrong, 'SYN-GOODS-02');
  equal(step.state.feedback.kind, 'ok');
  equal(countedTotal(step.state), 4);
});

test('不是单里的商品编码：去认（条码、平台商品码），认的时候先不收扫码', () => {
  const step = scanCode(checking(), '6901234567890');
  equal(step.state.phase, 'resolving');
  equal(step.state.pending, '6901234567890');
  deepEqual(step.effects, [{ kind: 'identify', order: order(), code: '6901234567890' }]);
  equal(step.cue, undefined);
  equal(countedTotal(step.state), 0);
  equal(acceptingScans(step.state), false);
  unchanged(step.state, scanCode(step.state, 'SYN-GOODS-01'));
  equal(referenceGoods(order()).goodsCode, 'SYN-GOODS-01');
});

test('认出来了：认成这张单里的货就记一件（认成参照货与认成另一种都算）', () => {
  const resolving = scanCode(checking(), '6901234567890').state;
  const same = goodsIdentified(resolving, '6901234567890', { kind: 'same', via: 'alias' });
  equal(same.state.phase, 'checking');
  equal(countedTotal(same.state), 1);
  equal(same.state.feedback.text, '对。SYN-GOODS-01 已扫 1 / 3。');
  equal(same.cue, 'ok');

  const other = goodsIdentified(resolving, '6901234567890', { kind: 'other', goodsCode: 'SYN-GOODS-02' });
  equal(countedTotal(other.state), 1);
  equal(other.state.feedback.text, '对。SYN-GOODS-02 已扫 1 / 2。');
  equal(other.cue, 'ok');
});

test('认出来了但不是这张单的货：整块红、不记数，说它认成了哪个商品', () => {
  const resolving = scanCode(checking(), '6901234567890').state;
  const step = goodsIdentified(resolving, '6901234567890', { kind: 'other', goodsCode: 'SYN-GOODS-77' });
  equal(step.state.phase, 'checking');
  equal(countedTotal(step.state), 0);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('认成商品 SYN-GOODS-77') && step.state.feedback.text.includes('不是这张单的货') && step.state.feedback.text.includes('没有记数'));
  equal(step.cue, 'wrong');
  equal(packScreen(step.state, QUEUE).headline, '这件不能记数');
});

test('扫到的就是一个别的商品的编码（不是这张单的货）：直说这是别的商品，不说「认成」它自己', () => {
  const resolving = scanCode(checking(), 'SYN-GOODS-77').state;
  const step = goodsIdentified(resolving, 'SYN-GOODS-77', { kind: 'other', goodsCode: 'SYN-GOODS-77' });
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.startsWith('扫到的是商品 SYN-GOODS-77，它不是这张单的货'));
  ok(!step.state.feedback.text.includes('认成'));
  equal(countedTotal(step.state), 0);
});

test('认出来了是单里的货但已经扫够：红、不记数', () => {
  const full = scanned(checking(), ['SYN-GOODS-02', 'SYN-GOODS-02']);
  const resolving = scanCode(full, '6901234567890').state;
  const step = goodsIdentified(resolving, '6901234567890', { kind: 'other', goodsCode: 'SYN-GOODS-02' });
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('已经扫够了'));
  equal(countedTotal(step.state), 2);

  const fullReference = scanCode(scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01']), '6901234567890').state;
  const same = goodsIdentified(fullReference, '6901234567890', { kind: 'same', via: 'alias' });
  equal(same.state.feedback.kind, 'wrong');
  equal(countedTotal(same.state), 3);
});

test('认不出这个码：整块红、不记数，话里说认不出，不说成别的', () => {
  const resolving = scanCode(checking(), 'NOT-A-GOODS').state;
  const step = goodsIdentified(resolving, 'NOT-A-GOODS', { kind: 'unknown' });
  equal(step.state.phase, 'checking');
  equal(countedTotal(step.state), 0);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('认不出这是什么商品') && step.state.feedback.text.includes('没有记数'));
  equal(step.cue, 'wrong');
});

test('没能认：红、不记数，带原因，话里说「没能认出」而不是「认不出」', () => {
  const resolving = scanCode(checking(), 'NOT-A-GOODS').state;
  const step = goodsIdentified(resolving, 'NOT-A-GOODS', { kind: 'failed', reason: '读商品资料没成：网络没通（断了）' });
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('没能认出 NOT-A-GOODS') && step.state.feedback.text.includes('网络没通') && step.state.feedback.text.includes('没有记数'));
  ok(!step.state.feedback.text.includes('认不出这是什么商品'));
});

test('认码的答复过期了：不动', () => {
  const resolving = scanCode(checking(), 'NOT-A-GOODS').state;
  unchanged(resolving, goodsIdentified(resolving, 'OTHER-CODE', { kind: 'same', via: 'code' }));
  const back = checking();
  unchanged(back, goodsIdentified(back, 'NOT-A-GOODS', { kind: 'same', via: 'code' }));
});

test('Esc：清掉扫错与没核成的那句话，别的不动；没有红时不动', () => {
  const full = scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01']);
  const wrong = scanCode(full, 'SYN-GOODS-01').state;
  const cleared = dismissWrong(wrong);
  equal(cleared.state.feedback.kind, 'waiting');
  equal(countedTotal(cleared.state), 3);
  unchanged(full, dismissWrong(full));

  const missed = orderFound(finding(), 'SYN-OUT-001', { kind: 'unverified', reason: '读出库单没成：网络没通（断了）' }).state;
  equal(dismissWrong(missed).state.feedback.kind, 'waiting');
  const busy = startPack(WH, 'OBR-1').state;
  unchanged(busy, dismissWrong(busy));
});

// —— 改数量 ——

test('改数量：各种货已扫的件数填 0 或正整数；填得不对说是哪种货', () => {
  const o = order();
  const key1 = `${G1.goodsCode}\u0000${G1.unit}`;
  const key2 = `${G2.goodsCode}\u0000${G2.unit}`;
  deepEqual(judgeCounts(o, { [key1]: '2', [key2]: ' 0 ' }), { ok: true, counts: { [key1]: 2, [key2]: 0 } });
  for (const bad of ['', 'a', '-1', '1.5', '1e2']) {
    const judged = judgeCounts(o, { [key1]: '2', [key2]: bad });
    ok(!judged.ok && judged.reason.includes('SYN-GOODS-02'), JSON.stringify(bad));
  }
  const tooMany = judgeCounts(o, { [key1]: '1000000', [key2]: '0' });
  ok(!tooMany.ok && tooMany.reason.includes('最多填到 999999'));
  const missing = judgeCounts(o, { [key1]: '2' });
  ok(!missing.ok);
});

test('改数量：改到合计等于该有的，像扫齐一样自动提交；改成比该有的多或少，不提交，留在逐件扫', () => {
  const key1 = `${G1.goodsCode}\u0000${G1.unit}`;
  const key2 = `${G2.goodsCode}\u0000${G2.unit}`;
  const base = checking();

  const equalNow = changeCounts(base, { [key1]: '3', [key2]: '2' });
  ok(equalNow.ok);
  if (!equalNow.ok) return;
  equal(equalNow.step.state.phase, 'check-submitting');
  deepEqual(equalNow.step.effects, [{ kind: 'submit-check', order: order(), observed: 5 }]);

  const fewer = changeCounts(base, { [key1]: '3', [key2]: '0' });
  ok(fewer.ok);
  if (!fewer.ok) return;
  equal(fewer.step.state.phase, 'checking');
  equal(countedTotal(fewer.step.state), 3);
  ok(fewer.step.state.feedback.text.includes('已扫 3 / 5'));
  deepEqual(fewer.step.effects, []);

  const more = changeCounts(base, { [key1]: '4', [key2]: '2' });
  ok(more.ok);
  if (!more.ok) return;
  equal(more.step.state.phase, 'checking');
  equal(countedTotal(more.step.state), 6);
  equal(packScreen(more.step.state, QUEUE).headline, '已经扫了 6 件，比该有的多');

  const bad = changeCounts(base, { [key1]: 'x', [key2]: '2' });
  ok(!bad.ok && bad.reason.includes('SYN-GOODS-01'));
  const outside = changeCounts(packing(), { [key1]: '3', [key2]: '2' });
  ok(!outside.ok);
});

// —— 数量不符，照实提交 ——

test('数量不符的情形：只在逐件扫、合计与该有的不同时有；多了、少了、一件没扫都算', () => {
  equal(mismatchPlan(checking()) === null, false, '一件没扫也是不符：照实记 0');
  deepEqual(mismatchPlan(checking()), { expected: 5, observed: 0, difference: 5, over: false });
  deepEqual(mismatchPlan(scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01'])), { expected: 5, observed: 3, difference: 2, over: false });

  const key1 = `${G1.goodsCode}\u0000${G1.unit}`;
  const key2 = `${G2.goodsCode}\u0000${G2.unit}`;
  const more = changeCounts(checking(), { [key1]: '4', [key2]: '2' });
  ok(more.ok);
  if (more.ok) deepEqual(mismatchPlan(more.step.state), { expected: 5, observed: 6, difference: 1, over: true });

  equal(mismatchPlan(packing()), null);
  equal(mismatchPlan(idle()), null);
  const submitting = scanned(checking(), ALL);
  equal(mismatchPlan(submitting), null);
});

test('确认框的话照核到的规则说：记下这一次复核、仍停在已拣货、不能打包、可以再复核', () => {
  const lines = mismatchLines({ expected: 5, observed: 3, difference: 2, over: false });
  equal(lines.length, 3);
  ok(lines[0].includes('已拣 5 件') && lines[0].includes('点到的是 3 件') && lines[0].includes('少了 2 件'));
  ok(lines[1].includes('记下来') && lines[1].includes('已拣货') && lines[1].includes('不能打包'));
  ok(lines[2].includes('再复核一次'));
  ok(mismatchLines({ expected: 5, observed: 6, difference: 1, over: true })[0].includes('多了 1 件'));
  ok(mismatchLines({ expected: 5, observed: 0, difference: 5, over: false })[0].includes('一件都没点到'));
});

test('照实提交：提交的是实点总数，记着该有几件、点到几件；对得上的不让走这条', () => {
  const three = scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01']);
  const step = submitMismatch(three);
  equal(step.state.phase, 'check-submitting');
  deepEqual(step.state.mismatch, { expected: 5, observed: 3 });
  deepEqual(step.state.submission, { step: 'check', order: order(), observed: 3 });
  deepEqual(step.effects, [{ kind: 'submit-check', order: order(), observed: 3 }]);

  const zero = submitMismatch(checking());
  deepEqual(zero.effects, [{ kind: 'submit-check', order: order(), observed: 0 }]);

  unchanged(packing(), submitMismatch(packing()));
  const submitting = scanned(checking(), ALL);
  unchanged(submitting, submitMismatch(submitting));
});

test('逐件扫这一步能点的按钮：改数量一直能点，照实提交只在合计不等于该有的时候能点', () => {
  const none = packActions(checking());
  equal(none.changeCount, true);
  equal(none.mismatch, true);
  equal(none.leave, true);
  equal(none.addParcel, false);
  equal(none.submitPack, false);
  const key1 = `${G1.goodsCode}\u0000${G1.unit}`;
  const key2 = `${G2.goodsCode}\u0000${G2.unit}`;
  const equalNow = changeCounts(checking(), { [key1]: '3', [key2]: '2' });
  ok(equalNow.ok);
  if (equalNow.ok) {
    const actions = packActions(equalNow.step.state);
    equal(actions.changeCount, false);
    equal(actions.mismatch, false);
    equal(actions.leave, false, '提交在途不让换单');
  }
});

// —— 复核的答复 ——

function checkSubmitting(): PackState {
  return scanned(checking(), ALL);
}

test('复核对上了（CHECKED）：进打包，备好第一个包裹，响扫对的声音，队列要重读', () => {
  const step = applyVerdict(checkSubmitting(), checkVerdict('CHECKED'));
  equal(step.state.phase, 'packing');
  equal(step.state.parcels.length, 1);
  equal(step.state.submission, null);
  equal(step.state.feedback.kind, 'ok');
  ok(step.state.feedback.text.includes('复核通过') && step.state.feedback.text.includes('5 件都对上了') && step.state.feedback.text.includes('接着打包'));
  deepEqual(step.effects, [{ kind: 'refresh-queue' }]);
  equal(step.cue, 'ok');
  equal(packProgress(step.state), '第 2 步，共 2 步：打包');
});

test('复核已有结果（EXISTING_RESULT）也算对上了：进打包，说明是以前记过的、这次没有重复记', () => {
  const verdict = checkVerdict('EXISTING_RESULT');
  equal(verdict.verdict, 'done');
  const step = applyVerdict(checkSubmitting(), verdict);
  equal(step.state.phase, 'packing');
  ok(step.state.feedback.text.includes('没有重复记') && step.state.feedback.text.includes('接着打包'));
});

test('复核不符（CHECK_MISMATCH）：这一次已经记下，这张单仍停在已拣货、不能打包；说明发生了什么，等换一张单', () => {
  const three = scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01']);
  const submitting = submitMismatch(three).state;
  const verdict = checkVerdict('CHECK_MISMATCH');
  equal(verdict.verdict, 'done', '后端答 CHECK_MISMATCH 是记下了，不是没收');
  const step = applyVerdict(submitting, verdict);
  equal(step.state.phase, 'mismatch');
  equal(step.state.feedback.kind, 'wrong');
  const text = step.state.feedback.text;
  ok(text.includes('该有 5 件') && text.includes('点到 3 件'));
  ok(text.includes('已经记下') && text.includes('仍停在「已拣货」') && text.includes('不能打包'));
  deepEqual(step.effects, [{ kind: 'refresh-queue' }]);
  equal(step.cue, 'wrong');
  const screen = packScreen(step.state, QUEUE);
  equal(screen.label, '复核不符');
  equal(screen.headline, '数量对不上，不能打包');
  equal(screen.counter, '该有 5 件，点到 3 件');
  equal(scanPlaceholder(step.state), '先点「换一张单」');
  equal(acceptingScans(step.state), false);
  unchanged(step.state, scanCode(step.state, 'SYN-GOODS-01'));
  const actions = packActions(step.state);
  equal(actions.leave, true);
  equal(actions.resend, false);
  equal(actions.addParcel, false);
});

test('复核不符之后换一张单：回到等扫出库单号，队列重读，屏上说那张单的复核已记下、仍停在已拣货', () => {
  const mismatch = applyVerdict(submitMismatch(scanned(checking(), ['SYN-GOODS-01'])).state, checkVerdict('CHECK_MISMATCH')).state;
  const step = leaveOrder(mismatch);
  equal(step.state.phase, 'waiting');
  equal(step.state.order, null);
  deepEqual(step.effects, [{ kind: 'refresh-queue' }]);
  ok(step.state.feedback.text.includes('复核不符已经记下') && step.state.feedback.text.includes('已拣货'));
  equal(packScreen(step.state, QUEUE).headline, '扫出库单号');
});

test('复核不符而屏上的数与后端核的数一样：说后端核出来的已拣数量与屏上的不一样，不写「该有 N 件点到 N 件」', () => {
  const text = mismatchText({ expected: 5, observed: 5 });
  ok(text.includes('与屏上的不一样') && text.includes('已经记下') && text.includes('不能打包'));
  const submitting = scanned(checking(), ALL);
  const step = applyVerdict(submitting, checkVerdict('CHECK_MISMATCH'));
  equal(step.state.phase, 'mismatch');
  ok(step.state.feedback.text.includes('与屏上的不一样'));
});

test('复核没记成（单没了、状态变了、仓对不上、没配置）：红，说原因，等换一张单；这些不是「再送一次」能解决的', () => {
  for (const outcome of ['STATUS_MISMATCH', 'WAVE_NOT_FOUND', 'CONTENT_INVALID', 'CONTENT_CONFLICT', 'NOT_CONFIGURED']) {
    const verdict = checkVerdict(outcome);
    equal(verdict.verdict, 'refused', outcome);
    const step = applyVerdict(checkSubmitting(), verdict);
    equal(step.state.phase, 'check-refused', outcome);
    equal(step.state.feedback.kind, 'wrong', outcome);
    ok(step.state.feedback.text.includes('没有记'), outcome);
    equal(step.cue, 'wrong');
    equal(packActions(step.state).resend, false);
    equal(packActions(step.state).leave, true);
    equal(scanPlaceholder(step.state), '先点「换一张单」');
  }
  const refused = applyVerdict(checkSubmitting(), judgeWorkFactAnswer({ kind: 'notGranted' }, checkOutcomeLabels));
  equal(refused.state.phase, 'check-refused');
  const unconfigured = applyVerdict(checkSubmitting(), judgeWorkFactAnswer({ kind: 'unconfigured' }, checkOutcomeLabels));
  equal(unconfigured.state.phase, 'check-refused');
  ok(unconfigured.state.feedback.text.includes('未配置'));
});

// Covers: 按货配出来的单复核时答状态不符（别的工位同时扫了同一种货、先复核了它）：不让人再扫，离开这张单、
// 用扫过的那个码另配一张；不是按货配的单、别的拒法、配的不是这张单，都照旧等「换一张单」。
test('按货配的单被别的工位先复核了：用同一个码另配一张', () => {
  const verdict = checkVerdict('STATUS_MISMATCH');
  const refused = applyVerdict(checkSubmitting(), verdict).state;
  const requestId = refused.order?.requestId ?? '';
  const again = rematchAfterRefusal(refused, verdict, { requestId, code: 'SKU-SCANNED' });
  ok(again !== null);
  if (again === null) return;
  equal(again.state.phase, 'finding');
  equal(again.state.pending, 'SKU-SCANNED');
  equal(again.state.order, null);
  ok(again.state.feedback.text.includes('别的工位刚复核过') && again.state.feedback.text.includes('另配一张单'));
  deepEqual(again.effects, [{ kind: 'refresh-queue' }, { kind: 'find-order', code: 'SKU-SCANNED' }]);

  equal(rematchAfterRefusal(refused, verdict, null), null);
  equal(rematchAfterRefusal(refused, verdict, { requestId: 'OBR-OTHER', code: 'SKU-SCANNED' }), null);
  const invalid = checkVerdict('CONTENT_INVALID');
  equal(rematchAfterRefusal(applyVerdict(checkSubmitting(), invalid).state, invalid, { requestId, code: 'SKU-SCANNED' }), null);
  equal(rematchAfterRefusal(checkSubmitting(), verdict, { requestId, code: 'SKU-SCANNED' }), null);
});

test('复核没送到：停在没送到，等「再送一次」；再送是同一笔（接线的一方保留那一次），成了才进打包', () => {
  const unsent = applyVerdict(checkSubmitting(), judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, checkOutcomeLabels));
  equal(unsent.state.phase, 'check-unsent');
  equal(unsent.state.feedback.kind, 'unsent');
  ok(unsent.state.feedback.text.includes('再送一次') && unsent.state.feedback.text.includes('同一个作业事实号'));
  equal(unsent.cue, 'wrong');
  const screen = packScreen(unsent.state, QUEUE);
  equal(screen.label, '没送到');
  equal(screen.headline, '这一笔复核没有送到');
  equal(screen.counter, '实点 5 件，用同一个作业事实号再送。');
  equal(packActions(unsent.state).resend, true);
  equal(scanPlaceholder(unsent.state), '先点「再送一次」');
  unchanged(unsent.state, scanCode(unsent.state, 'SYN-GOODS-01'));

  const again = resend(unsent.state);
  equal(again.state.phase, 'check-submitting');
  deepEqual(again.effects, [{ kind: 'resend' }]);
  equal(again.state.submission?.step, 'check');
  const done = applyVerdict(again.state, checkVerdict('EXISTING_RESULT'));
  equal(done.state.phase, 'packing');

  const noAnswer = applyVerdict(checkSubmitting(), judgeWorkFactAnswer({ kind: 'noAnswer', status: 503, code: 'UNAVAILABLE' }, checkOutcomeLabels));
  equal(noAnswer.state.phase, 'check-unsent');
});

test('答复对不上现在的阶段（没有在途的提交、阶段不对）：不动', () => {
  unchanged(checking(), applyVerdict(checking(), checkVerdict('CHECKED')));
  const packingState = packing();
  unchanged(packingState, applyVerdict(packingState, packVerdict('PACKED')));
  unchanged(checking(), resend(checking()));
});

// —— 打包：包裹 ——

test('打包的第一屏：指着第一个包裹说扫包材码，小字是已装进包裹几件', () => {
  const screen = packScreen(packing(), QUEUE);
  equal(screen.label, '下一步');
  equal(screen.headline, '给包裹 1 扫包材码（或点选）');
  equal(screen.counter, '已装进包裹 3 / 5 件，1 个包裹');
  deepEqual(packPacked(packing()), { assigned: 3, expected: 5 });
  equal(acceptingScans(packing()), true);
  equal(scanPlaceholder(packing()), '扫包材码，或敲进去按回车');
});

test('再加一个包裹：新包裹先装第一种还没装满的货，差几件装几件；都装满了留空件数', () => {
  const two = addParcel(packing());
  equal(two.state.parcels.length, 2);
  equal(two.state.parcels[1].goodsCode, 'SYN-GOODS-02');
  equal(two.state.parcels[1].quantity, '2');
  equal(two.state.feedback.text, '加了包裹 2。');
  equal(two.state.parcels[0].key !== two.state.parcels[1].key, true);
  deepEqual(packPacked(two.state), { assigned: 5, expected: 5 });

  const three = addParcel(two.state);
  equal(three.state.parcels.length, 3);
  equal(three.state.parcels[2].goodsCode, 'SYN-GOODS-01');
  equal(three.state.parcels[2].quantity, '', '装满了就留空，不替人定从哪个包裹匀');

  unchanged(checking(), addParcel(checking()));
});

test('删包裹：按本地标识删；没有这个标识不动；删光了提示再加一个', () => {
  const two = addParcel(packing()).state;
  const first = two.parcels[0].key;
  const removed = removeParcel(two, first);
  equal(removed.state.parcels.length, 1);
  equal(removed.state.parcels[0].goodsCode, 'SYN-GOODS-02');
  unchanged(two, removeParcel(two, 999));
  const none = removeParcel(removed.state, removed.state.parcels[0].key).state;
  equal(none.parcels.length, 0);
  equal(packHeadline(none), '点「再加一个包裹」');
  equal(packActions(none).submitPack, false);
  unchanged(checking(), removeParcel(checking(), 1));
});

test('改包裹上的格：重量、重量单位、件数、选哪种货；改了尺寸就记下「人改过」', () => {
  const base = packing();
  const key = base.parcels[0].key;
  const weight = editParcel(base, key, { weight: '1.25', weightUnit: 'KG' }).state.parcels[0];
  equal(weight.weight, '1.25');
  equal(weight.weightUnit, 'KG');
  equal(weight.dimsEdited, false);

  const dims = editParcel(base, key, { length: '31' }).state.parcels[0];
  equal(dims.length, '31');
  equal(dims.dimsEdited, true);

  const goods = editParcel(base, key, { goodsCode: 'SYN-GOODS-02', unit: '件', quantity: '2' }).state.parcels[0];
  deepEqual([goods.goodsCode, goods.unit, goods.quantity], ['SYN-GOODS-02', '件', '2']);

  unchanged(base, editParcel(base, 999, { weight: '9' }));
  unchanged(checking(), editParcel(checking(), key, { weight: '9' }));
});

test('点选包材：落在指定的包裹上，尺寸取包材的外廓并说一声；没登记外廓的不说、尺寸留空；停用的、没这个包裹的不收', () => {
  const base = addParcel(packing()).state;
  const [one, two] = base.parcels;
  const picked = chooseMaterial(base, one.key, BOX);
  const parcel = picked.state.parcels[0];
  deepEqual([parcel.packaging, parcel.packagingName, parcel.length, parcel.width, parcel.height, parcel.dimensionUnit], ['SYN-BOX-S', '小号纸箱', '30', '20', '15', 'CM']);
  equal(parcel.weight, '', '重量不替人填');
  equal(parcel.weightUnit, '', '重量单位也不替人填');
  equal(picked.state.feedback.kind, 'ok');
  ok(picked.state.feedback.text.includes('包裹 1') && picked.state.feedback.text.includes('小号纸箱') && picked.state.feedback.text.includes('外廓'));
  equal(picked.state.parcels[1].packaging, '', '别的包裹不受影响');

  const bag = chooseMaterial(base, two.key, BAG);
  deepEqual([bag.state.parcels[1].length, bag.state.parcels[1].dimensionUnit], ['', '']);
  ok(!bag.state.feedback.text.includes('外廓'));

  unchanged(base, chooseMaterial(base, one.key, { ...BOX, deactivated: true }));
  unchanged(base, chooseMaterial(base, 999, BOX));
  unchanged(checking(), chooseMaterial(checking(), 1, BOX));
});

test('人改过的尺寸，换包材时不被覆盖', () => {
  const base = packing();
  const key = base.parcels[0].key;
  const edited = editParcel(chooseMaterial(base, key, BOX).state, key, { length: '35' }).state;
  const swapped = chooseMaterial(edited, key, { ...BOX, code: 'SYN-BOX-L', length: '60' }).state.parcels[0];
  equal(swapped.packaging, 'SYN-BOX-L');
  equal(swapped.length, '35');
});

// —— 打包：扫包材码 ——

test('扫到包材码：去核登没登记、在不在用，核的时候先不收扫码', () => {
  const step = scanCode(packing(), 'SYN-BOX-S');
  equal(step.state.phase, 'identifying');
  equal(step.state.pending, 'SYN-BOX-S');
  deepEqual(step.effects, [{ kind: 'identify-packaging', code: 'SYN-BOX-S' }]);
  equal(step.cue, undefined);
  equal(acceptingScans(step.state), false);
  unchanged(step.state, scanCode(step.state, 'SYN-BOX-L'));
});

test('这一步扫到商品码或出库单号：红、不去核、什么都没记', () => {
  const goods = scanCode(packing(), 'SYN-GOODS-01');
  equal(goods.state.phase, 'packing');
  equal(goods.state.feedback.kind, 'wrong');
  ok(goods.state.feedback.text.includes('要扫包材码') && goods.state.feedback.text.includes('没有记任何东西'));
  deepEqual(goods.effects, []);
  equal(goods.cue, 'wrong');

  for (const code of ['SYN-OUT-001', 'OBR-1']) {
    const number = scanCode(packing(), code);
    equal(number.state.feedback.kind, 'wrong', code);
    ok(number.state.feedback.text.includes('出库单的单号'), code);
    deepEqual(number.effects, []);
  }
  const screen = packScreen(goods.state, QUEUE);
  equal(screen.label, '扫错了');
  equal(screen.headline, '这个码不能用');
});

test('每个包裹都选好包材了再扫包材码：红，说要换点「换包材」，不去核', () => {
  const base = packing();
  const chosen = chooseMaterial(base, base.parcels[0].key, BOX).state;
  const step = scanCode(chosen, 'SYN-BOX-L');
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('换包材'));
  deepEqual(step.effects, []);
  equal(scanTarget(chosen), null);
});

test('包材核到了：落在第一个还没选包材的包裹上；再扫一个落在下一个包裹上', () => {
  const two = addParcel(packing()).state;
  equal(scanTarget(two), two.parcels[0].key);
  const identifying = scanCode(two, 'SYN-BOX-S').state;
  const first = packagingIdentified(identifying, 'SYN-BOX-S', { kind: 'found', material: BOX });
  equal(first.state.phase, 'packing');
  equal(first.state.parcels[0].packaging, 'SYN-BOX-S');
  equal(first.state.parcels[1].packaging, '');
  equal(first.state.feedback.kind, 'ok');
  ok(first.state.feedback.text.startsWith('对。') && first.state.feedback.text.includes('包裹 1'));
  equal(first.state.feedback.scanned, 'SYN-BOX-S');
  equal(first.cue, 'ok');
  equal(scanTarget(first.state), two.parcels[1].key);

  const second = packagingIdentified(scanCode(first.state, 'SYN-BAG-1').state, 'SYN-BAG-1', { kind: 'found', material: BAG });
  equal(second.state.parcels[1].packaging, 'SYN-BAG-1');
  deepEqual([second.state.parcels[1].length, second.state.parcels[1].dimensionUnit], ['', ''], '没登记外廓的，尺寸留给人填');
});

test('包材核出来是已停用、没有、没核成：各说各的，什么都不记；没核成不说成没有', () => {
  const identifying = scanCode(packing(), 'SYN-BOX-S').state;
  const old = packagingIdentified(identifying, 'SYN-BOX-S', { kind: 'found', material: { ...BOX, deactivated: true } });
  equal(old.state.phase, 'packing');
  equal(old.state.feedback.kind, 'wrong');
  ok(old.state.feedback.text.includes('已停用') && old.state.feedback.text.includes('没有记下'));
  equal(old.state.parcels[0].packaging, '');
  equal(old.cue, 'wrong');

  const absent = packagingIdentified(identifying, 'SYN-BOX-S', { kind: 'absent' });
  equal(absent.state.feedback.kind, 'wrong');
  ok(absent.state.feedback.text.includes('登记册里没有包材 SYN-BOX-S'));

  const unverified = packagingIdentified(identifying, 'SYN-BOX-S', { kind: 'unverified', reason: '读包材名册没成：网络没通（断了）' });
  equal(unverified.state.feedback.kind, 'unsent');
  ok(unverified.state.feedback.text.includes('没能核对包材') && unverified.state.feedback.text.includes('网络没通'));
  ok(!unverified.state.feedback.text.includes('登记册里没有'));
  equal(unverified.state.parcels[0].packaging, '');
});

test('包材核的当口人已经点选了：不覆盖，红着说每个包裹都选好了；答复过期了不动', () => {
  const identifying = scanCode(packing(), 'SYN-BOX-S').state;
  const picked = chooseMaterial({ ...identifying, phase: 'packing' }, identifying.parcels[0].key, BOX).state;
  const racing = { ...picked, phase: 'identifying' as PackPhase, pending: 'SYN-BOX-S' };
  const step = packagingIdentified(racing, 'SYN-BOX-S', { kind: 'found', material: { ...BOX, code: 'SYN-BOX-L' } });
  equal(step.state.parcels[0].packaging, 'SYN-BOX-S');
  equal(step.state.feedback.kind, 'wrong');

  unchanged(identifying, packagingIdentified(identifying, 'SYN-OTHER', { kind: 'absent' }));
  const back = packing();
  unchanged(back, packagingIdentified(back, 'SYN-BOX-S', { kind: 'absent' }));
});

// —— 打包：下一步指哪里 ——

test('打包的屏一步一步指：包材 → 重量 → 重量单位 → 长宽高 → 尺寸单位 → 装齐 → 提交', () => {
  let state = packing();
  const key = state.parcels[0].key;
  equal(packHeadline(state), '给包裹 1 扫包材码（或点选）');
  state = chooseMaterial(state, key, BAG).state;
  equal(packHeadline(state), '给包裹 1 录重量');
  state = editParcel(state, key, { weight: '1.25' }).state;
  equal(packHeadline(state), '给包裹 1 填重量单位');
  state = editParcel(state, key, { weightUnit: 'KG' }).state;
  equal(packHeadline(state), '给包裹 1 填长宽高');
  state = editParcel(state, key, { length: '30', width: '20', height: '15' }).state;
  equal(packHeadline(state), '给包裹 1 填尺寸单位');
  state = editParcel(state, key, { dimensionUnit: 'CM' }).state;
  equal(packHeadline(state), 'SYN-GOODS-02 还有 2 件没装进包裹');
  state = addParcel(state).state;
  equal(packHeadline(state), '给包裹 2 扫包材码（或点选）');
  equal(packActions(state).submitPack, false);
});

test('装多了的提示与装齐的提示', () => {
  const state = packing();
  const key = state.parcels[0].key;
  let filled = editParcel(chooseMaterial(state, key, BOX).state, key, { weight: '1', weightUnit: 'KG', quantity: '4' }).state;
  equal(packHeadline(filled), 'SYN-GOODS-01 装多了 1 件');
  equal(packActions(filled).submitPack, false);
  filled = editParcel(filled, key, { quantity: '3' }).state;
  equal(packHeadline(filled), 'SYN-GOODS-02 还有 2 件没装进包裹');
  equal(packHeadline(idle()), '');
});

// —— 打包：提交 ——

test('填齐了才能提交；提交的是整张单（所有包裹一次交出去），屏上写几个包裹', () => {
  const half = packing();
  unchanged(half, submitPack(half));
  unchanged(idle(), submitPack(idle()));

  const ready = readyToPack();
  equal(ready.parcels.length, 2);
  equal(packHeadline(ready), '都填好了，提交打包');
  equal(packActions(ready).submitPack, true);
  const step = submitPack(ready);
  equal(step.state.phase, 'pack-submitting');
  equal(step.effects.length, 1);
  const effect = step.effects[0];
  equal(effect.kind, 'submit-pack');
  ok(effect.kind === 'submit-pack' && effect.parcels.length === 2 && effect.order.requestId === 'OBR-1');
  equal(step.state.submission?.step, 'pack');
  const screen = packScreen(step.state, QUEUE);
  equal(screen.headline, '正在提交打包…');
  equal(screen.counter, 'SYN-OUT-001，2 个包裹');
  equal(acceptingScans(step.state), false);
  equal(packActions(step.state).leave, false);
  unchanged(step.state, scanCode(step.state, 'SYN-BOX-S'));
});

function packSubmitting(): PackState {
  return submitPack(readyToPack()).state;
}

test('打包成了（PACKED 与 SUBMITTED）：回到等扫出库单号，这一趟打包好了几张加一，队列重读，响扫对的声音', () => {
  for (const outcome of ['PACKED', 'SUBMITTED']) {
    const verdict = packVerdict(outcome);
    equal(verdict.verdict, 'done', outcome);
    const step = applyVerdict(packSubmitting(), verdict);
    equal(step.state.phase, 'waiting', outcome);
    equal(step.state.order, null);
    deepEqual(step.state.parcels, []);
    equal(step.state.done, 1);
    equal(step.state.feedback.kind, 'ok');
    ok(step.state.feedback.text.includes('SYN-OUT-001 打包好了：2 个包裹'), outcome);
    deepEqual(step.effects, [{ kind: 'refresh-queue' }]);
    equal(step.cue, 'ok');
    equal(packScreen(step.state, QUEUE).headline, '扫出库单号');
    ok(packScreen(step.state, QUEUE).counter.includes('这一趟打包好了 1 张'));
  }
  ok(applyVerdict(packSubmitting(), packVerdict('SUBMITTED')).state.feedback.text.includes('委托'));
  equal(packProgress(applyVerdict(packSubmitting(), packVerdict('PACKED')).state), '这一趟打包了 1 张');
});

test('打包已有结果（EXISTING_RESULT）也算成：说明是以前记过的、这次没有重复记', () => {
  const step = applyVerdict(packSubmitting(), packVerdict('EXISTING_RESULT'));
  equal(step.state.phase, 'waiting');
  equal(step.state.done, 1);
  ok(step.state.feedback.text.includes('没有重复记'));
});

test('打包没记成（内容不合）：红，说没有记，包裹都还在，回去改或换一张单', () => {
  const verdict = packVerdict('CONTENT_INVALID');
  equal(verdict.verdict, 'refused');
  const step = applyVerdict(packSubmitting(), verdict);
  equal(step.state.phase, 'pack-refused');
  equal(step.state.parcels.length, 2);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('没有记'));
  equal(step.cue, 'wrong');
  equal(packScreen(step.state, QUEUE).counter, '包裹都还在，可以改一改再提交。');
  equal(scanPlaceholder(step.state), '先点「回去改」');
  equal(packActions(step.state).backToForm, true);
  equal(packActions(step.state).resend, false);
  equal(packActions(step.state).leave, true);
});

test('打包答单状态不对、单找不到、内容冲突：这几个词提交委托那一步也会答，有没有记下说不准，话里只说答复能证明的，让人重新扫这张单看状态', () => {
  for (const outcome of ['STATUS_MISMATCH', 'WAVE_NOT_FOUND', 'CONTENT_CONFLICT']) {
    const verdict = packVerdict(outcome);
    equal(verdict.verdict, 'refused', outcome);
    equal(packResendable.has(outcome), false, `${outcome} 重送也不会变，不给「再送一次」`);
    const step = applyVerdict(packSubmitting(), verdict);
    equal(step.state.phase, 'pack-refused', outcome);
    equal(step.state.parcels.length, 2);
    equal(step.state.feedback.kind, 'wrong');
    ok(step.state.feedback.text.includes('重新扫这张单'), outcome);
    ok(!step.state.feedback.text.includes('这一笔没有记。'), `${outcome} 不能断言没记下`);
    equal(step.cue, 'wrong');
    equal(packActions(step.state).resend, false);
  }
  const refused = applyVerdict(packSubmitting(), judgeWorkFactAnswer({ kind: 'loginRequired' }, packOutcomeLabels));
  equal(refused.state.phase, 'pack-refused');
  const unconfigured = applyVerdict(packSubmitting(), judgeWorkFactAnswer({ kind: 'unconfigured' }, packOutcomeLabels));
  equal(unconfigured.state.phase, 'pack-refused', '接入没配置是没送进业务的，不是「打包记下了」');
});

test('回去改：包裹草稿原样在，改了再提交；不是没记成的阶段不动', () => {
  const refused = applyVerdict(packSubmitting(), packVerdict('CONTENT_INVALID')).state;
  const back = backToForm(refused);
  equal(back.state.phase, 'packing');
  equal(back.state.parcels.length, 2);
  equal(back.state.submission, null);
  equal(packActions(back.state).submitPack, true);
  unchanged(packing(), backToForm(packing()));
});

test('打包记下了但委托没提交成：不当成没记成，也不当成做完；等「再送一次」接着提交，这一张不算做完', () => {
  // 前四个词只有提交委托那一步才会答，答到它们就证明打包已经记下；未配置分不出是哪一步，说话留余地
  const rows: Array<[string, boolean]> = [
    ['OWNERSHIP_UNRESOLVED', true],
    ['ADMISSION_PAUSED', true],
    ['OTHER_PRODUCTION_AUTHORITY', true],
    ['INPUT_NOT_ACCEPTED', true],
    ['NOT_CONFIGURED', false],
  ];
  for (const [outcome, recorded] of rows) {
    ok(packResendable.has(outcome), outcome);
    equal(packCommissionWords.has(outcome), recorded, outcome);
    const verdict = packVerdict(outcome);
    equal(verdict.verdict, 'refused', outcome);
    const step = applyVerdict(packSubmitting(), verdict);
    equal(step.state.phase, 'pack-pending', outcome);
    equal(step.state.recorded, recorded, outcome);
    equal(step.state.done, 0, outcome);
    equal(step.state.parcels.length, 2);
    equal(step.state.feedback.kind, 'unsent');
    ok(step.state.feedback.text.length > 10);
    deepEqual(step.effects, [{ kind: 'refresh-queue' }]);
    equal(step.cue, 'wrong');
    const screen = packScreen(step.state, QUEUE);
    equal(screen.label, recorded ? '委托没提交' : '没走完', outcome);
    equal(screen.headline, recorded ? '打包记下了，委托还没提交' : '这一笔打包还没有走完', outcome);
    ok(screen.counter.includes('换了单，这个工位就不能再送这一笔'), outcome);
    equal(packActions(step.state).resend, true);
    equal(packActions(step.state).leave, true);
    equal(scanPlaceholder(step.state), '先点「再送一次」');
  }
  const pending = applyVerdict(packSubmitting(), packVerdict('OWNERSHIP_UNRESOLVED'));
  ok(pending.state.feedback.text.includes('打包已经记下') && pending.state.feedback.text.includes('再送一次'));
  const unsure = applyVerdict(packSubmitting(), packVerdict('NOT_CONFIGURED'));
  ok(!unsure.state.feedback.text.startsWith('打包已经记下'), '未配置不断言打包记下了');
  ok(unsure.state.feedback.text.includes('重新扫这张单'));
  ok(leaveOrder(unsure.state).state.feedback.text.includes('不知道记下了没有'));
  ok(leaveOrder(pending.state).state.feedback.text.includes('打包已经记下'));

  const again = resend(pending.state);
  equal(again.state.phase, 'pack-submitting');
  deepEqual(again.effects, [{ kind: 'resend' }]);
  const done = applyVerdict(again.state, packVerdict('SUBMITTED'));
  equal(done.state.phase, 'waiting');
  equal(done.state.done, 1, '委托提交成了才算这一张做完，只算一次');
});

test('打包没送到：停在没送到，包裹都在，等「再送一次」，用同一个事实号', () => {
  const unsent = applyVerdict(packSubmitting(), judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, packOutcomeLabels));
  equal(unsent.state.phase, 'pack-unsent');
  equal(unsent.state.parcels.length, 2);
  equal(unsent.state.feedback.kind, 'unsent');
  equal(packScreen(unsent.state, QUEUE).label, '没送到');
  const again = resend(unsent.state);
  equal(again.state.phase, 'pack-submitting');
  deepEqual(again.effects, [{ kind: 'resend' }]);
  const done = applyVerdict(again.state, packVerdict('EXISTING_RESULT'));
  equal(done.state.phase, 'waiting');
  equal(done.state.done, 1);
});

// —— 换一张单 ——

test('换一张单：能换的阶段回到等扫出库单号，丢掉没记下的数与包裹，屏上说清；队列重读；这一趟做完几张留着', () => {
  const phases: Array<[string, PackState, string]> = [
    ['checking 一件没扫', checking(), '换了一张单。'],
    ['checking 扫到一半', scanned(checking(), ['SYN-GOODS-01']), '扫到一半的数没有记下'],
    ['check-unsent', applyVerdict(checkSubmitting(), judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, checkOutcomeLabels)).state, '没有收到后端的确认'],
    ['check-refused', applyVerdict(checkSubmitting(), checkVerdict('STATUS_MISMATCH')).state, '没有记成'],
    ['packing', packing(), '填到一半的包裹没有记下'],
    ['pack-unsent', applyVerdict(packSubmitting(), judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, packOutcomeLabels)).state, '没有收到后端的确认'],
    ['pack-pending', applyVerdict(packSubmitting(), packVerdict('ADMISSION_PAUSED')).state, '委托没有提交'],
    ['pack-refused', applyVerdict(packSubmitting(), packVerdict('CONTENT_INVALID')).state, '没有记成'],
  ];
  for (const [name, state, phrase] of phases) {
    equal(packActions(state).leave, true, name);
    const step = leaveOrder({ ...state, done: 2 });
    equal(step.state.phase, 'waiting', name);
    equal(step.state.order, null, name);
    deepEqual(step.state.parcels, [], name);
    deepEqual(step.state.counted, {}, name);
    equal(step.state.submission, null, name);
    equal(step.state.done, 2, name);
    equal(step.state.feedback.kind, 'waiting', name);
    ok(step.state.feedback.text.includes(phrase), `${name}：${step.state.feedback.text}`);
    deepEqual(step.effects, [{ kind: 'refresh-queue' }], name);
    equal(step.cue, undefined);
  }
});

test('换一张单：读口与提交在途的阶段不让换，一笔请求发出去了，结果要等它回来', () => {
  const inFlight: Array<[string, PackState]> = [
    ['finding', finding()],
    ['opening', startPack(WH, 'OBR-1').state],
    ['resolving', scanCode(checking(), '6901234567890').state],
    ['check-submitting', checkSubmitting()],
    ['identifying', scanCode(packing(), 'SYN-BOX-S').state],
    ['pack-submitting', packSubmitting()],
    ['waiting', idle()],
  ];
  for (const [name, state] of inFlight) {
    equal(packActions(state).leave, false, name);
    unchanged(state, leaveOrder(state));
  }
});

// —— 按钮、扫描框、进度 ——

test('每个阶段扫描框收不收码、写什么字、进度写什么', () => {
  const rows: Array<[PackPhase, boolean, string, string]> = [
    ['waiting', true, '扫出库单号，或敲进去按回车', ''],
    ['finding', false, '正在处理，先不收扫码', ''],
    ['opening', false, '正在处理，先不收扫码', ''],
    ['checking', true, '扫商品码，或敲进去按回车', '第 1 步，共 2 步：复核'],
    ['resolving', false, '正在处理，先不收扫码', '第 1 步，共 2 步：复核'],
    ['check-submitting', false, '正在处理，先不收扫码', '第 1 步，共 2 步：复核'],
    ['check-unsent', false, '先点「再送一次」', '第 1 步，共 2 步：复核'],
    ['check-refused', false, '先点「换一张单」', '第 1 步，共 2 步：复核'],
    ['mismatch', false, '先点「换一张单」', '第 1 步，共 2 步：复核'],
    ['packing', true, '扫包材码，或敲进去按回车', '第 2 步，共 2 步：打包'],
    ['identifying', false, '正在处理，先不收扫码', '第 2 步，共 2 步：打包'],
    ['pack-submitting', false, '正在处理，先不收扫码', '第 2 步，共 2 步：打包'],
    ['pack-unsent', false, '先点「再送一次」', '第 2 步，共 2 步：打包'],
    ['pack-pending', false, '先点「再送一次」', '第 2 步，共 2 步：打包'],
    ['pack-refused', false, '先点「回去改」', '第 2 步，共 2 步：打包'],
  ];
  for (const [phase, accepting, placeholder, progress] of rows) {
    const state: PackState = { ...checking(), phase };
    equal(acceptingScans(state), accepting, phase);
    equal(scanPlaceholder(state), placeholder, phase);
    equal(packProgress({ ...state, done: 0 }), progress, phase);
  }
});

test('每个阶段能点的按钮：换单只在没有请求在途时能点，再送一次只在没送到与委托没提交时能点，刷新队列只在等扫出库单号', () => {
  const base = checking();
  const flags = (phase: PackPhase) => packActions({ ...base, phase });
  equal(flags('waiting').refresh, true);
  equal(flags('checking').refresh, false);
  equal(flags('packing').addParcel, true);
  equal(flags('checking').addParcel, false);
  for (const phase of ['check-unsent', 'pack-unsent', 'pack-pending'] as const) equal(flags(phase).resend, true, phase);
  for (const phase of ['waiting', 'checking', 'packing', 'mismatch', 'pack-refused', 'check-refused'] as const) equal(flags(phase).resend, false, phase);
  equal(packActions({ ...idle(), phase: 'checking' }).leave, false, '没有在做的单就没有可换的');
});

// —— 对后端答复的说法 ——

test('两张说法表：用到的每个答复都归对了类，话里没有英文答复名', () => {
  const doneForCheck = ['CHECKED', 'CHECK_MISMATCH', 'EXISTING_RESULT'];
  for (const outcome of Object.keys(checkOutcomeLabels)) {
    const verdict = checkVerdict(outcome);
    equal(verdict.verdict, doneForCheck.includes(outcome) ? 'done' : 'refused', `复核 ${outcome}`);
    equal(verdict.headline, checkOutcomeLabels[outcome]);
    ok(!/[A-Z]{2,}_[A-Z_]+/.test(verdict.headline), `复核 ${outcome} 的话里不带英文答复名`);
  }
  const doneForPack = ['PACKED', 'SUBMITTED', 'EXISTING_RESULT'];
  for (const outcome of Object.keys(packOutcomeLabels)) {
    const verdict = packVerdict(outcome);
    equal(verdict.verdict, doneForPack.includes(outcome) ? 'done' : 'refused', `打包 ${outcome}`);
    equal(verdict.headline, packOutcomeLabels[outcome]);
    ok(!/[A-Z]{2,}_[A-Z_]+/.test(verdict.headline), `打包 ${outcome} 的话里不带英文答复名`);
  }
  for (const outcome of packResendable) ok(outcome in packOutcomeLabels, `${outcome} 在打包说法表里要有话`);
});

// —— 从头走一遍 ——

test('整条路：扫出库单号 → 逐件扫 → 自动复核 → 扫包材码、录重量 → 提交 → 回到等扫，一张打包好了', () => {
  const o = order();
  let state = idle();
  state = scanCode(state, o.number).state;
  state = orderFound(state, o.number, { kind: 'found', row: rowOf(o) }).state;
  state = orderOpened(state, o.requestId, opened(o)).state;
  equal(state.phase, 'checking');
  state = scanned(state, ALL);
  equal(state.phase, 'check-submitting');
  state = applyVerdict(state, checkVerdict('CHECKED')).state;
  equal(state.phase, 'packing');

  state = addParcel(state).state;
  state = scanCode(state, 'SYN-BOX-S').state;
  state = packagingIdentified(state, 'SYN-BOX-S', { kind: 'found', material: BOX }).state;
  state = scanCode(state, 'SYN-BOX-S').state;
  state = packagingIdentified(state, 'SYN-BOX-S', { kind: 'found', material: BOX }).state;
  for (const parcel of state.parcels) state = editParcel(state, parcel.key, { weight: '1.25', weightUnit: 'KG' }).state;
  equal(packActions(state).submitPack, true);
  state = submitPack(state).state;
  equal(state.phase, 'pack-submitting');
  state = applyVerdict(state, packVerdict('SUBMITTED')).state;
  equal(state.phase, 'waiting');
  equal(state.done, 1);
  equal(state.order, null);
});

test('整条路（复核不符）：少扫两件，照实提交，后端记下不符，这张单停在已拣，回到等扫', () => {
  let state = scanned(checking(), ['SYN-GOODS-01', 'SYN-GOODS-01', 'SYN-GOODS-01']);
  const plan = mismatchPlan(state);
  ok(plan !== null && plan.difference === 2);
  state = submitMismatch(state).state;
  state = applyVerdict(state, checkVerdict('CHECK_MISMATCH')).state;
  equal(state.phase, 'mismatch');
  state = leaveOrder(state).state;
  equal(state.phase, 'waiting');
  equal(state.done, 0);
});
