import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { buildGuidedPayload } from '../guided-forms';
import { workFactPrefillFor, workFactSpecs } from '../work-fact';
import {
  acceptingScans,
  applyVerdict,
  beginPick,
  changeCount,
  changeStaging,
  dismissWrong,
  goodsIdentified,
  judgeQuantity,
  nextCell,
  pickActions,
  pickFactValues,
  pickOutcomeLabels,
  pickProgress,
  pickScreen,
  queueNotes,
  reloadQueue,
  resend,
  scanCode,
  scanPlaceholder,
  shortPickLines,
  shortPickPlan,
  skipCell,
  stagingVerified,
  startPick,
  submitShortPick,
  taskGoneAfter,
  type PickState,
  type PickStep,
} from './pick-flow';
import type { PickCell } from './pick-queue';
import type { StagingVerdict } from './pick-staging-check';
import { judgeWorkFactAnswer, prepareWorkFact } from './work-fact-submit';

function cell(taskId: string, locationCode: string, goodsCode = 'SYN-SKU-EARBUD-01', quantity = 5): PickCell {
  return { taskId, waveId: 'W-1', requestId: `REQ-${taskId}`, locationCode, goodsCode, unit: '件', stockState: 'GOOD', quantity };
}

const A = cell('T-A', 'A-01-03', 'SYN-SKU-EARBUD-01', 5);
const B = cell('T-B', 'A-01-05', 'SYN-SKU-CHARGER-02', 3);
const C = cell('T-C', 'B-01-02', 'SYN-SKU-MASK-03', 1);

// 核放货位的结果由读口决定（pick-staging-check.test.ts 钉各种情形的话与读法）；状态机这边只管拿到结果之后怎么走，所以直接造结果。
const OK: StagingVerdict = { kind: 'ok' };
const rejectedBecause = (reason: string): StagingVerdict => ({ kind: 'rejected', reason });
const unverifiedBecause = (reason: string): StagingVerdict => ({ kind: 'unverified', reason });

const STAGING = 'SYN-LOC-OUT-01';
const NOT_STAGING_ZONE = rejectedBecause(
  '库位 SYN-LOC-STORE-01 所在的库区 SYN-ZONE-STORE 用途是「存储」，不是待出库库区，拣出来的货不能放那里。',
);
const NO_NETWORK = unverifiedBecause('读库位名册没成：网络没通（断了）');

/** 在等放货位的状态上扫一个码，核的答复也回来了。 */
function scanStaging(before: PickState, code: string, verdict: StagingVerdict): PickStep {
  return stagingVerified(scanCode(before, code).state, code, verdict);
}

/** 已有放货位、站在队首这一格的库位上。 */
function atLocation(cells: PickCell[] = [A, B, C]): PickState {
  return startPick({ cells, staging: STAGING });
}

/** 已扫对库位，等商品。 */
function atGoods(cells: PickCell[] = [A, B, C]): PickState {
  return scanCode(atLocation(cells), cells[0].locationCode).state;
}

/** 已扫 n 件商品（直接用商品编码）。 */
function counted(n: number, cells: PickCell[] = [A, B, C]): PickState {
  let state = atGoods(cells);
  for (let i = 0; i < n; i += 1) state = scanCode(state, cells[0].goodsCode).state;
  return state;
}

function unchanged(before: PickState, step: PickStep) {
  deepEqual(step.state, before);
  deepEqual(step.effects, []);
  equal(step.cue, undefined);
}

// —— 放货位 ——

test('开工：没有放货位就先等放货位；已有核过的放货位就直接去等库位；队里没有格就是做完了', () => {
  equal(startPick({ cells: [A, B], staging: null }).phase, 'staging');
  equal(startPick({ cells: [A, B], staging: STAGING }).phase, 'location');
  equal(startPick({ cells: [], staging: STAGING }).phase, 'finished');
  equal(startPick({ cells: [], staging: null }).phase, 'staging');
});

test('放货位：扫到一个码先去核，进核对中，什么都还没定下，也不响', () => {
  const before = startPick({ cells: [A, B], staging: null });
  const step = scanCode(before, STAGING);
  equal(step.state.phase, 'verifying');
  equal(step.state.staging, null);
  deepEqual(step.state.verifying, { code: STAGING, remembered: false });
  equal(step.state.feedback.kind, 'waiting');
  equal(step.state.feedback.scanned, STAGING);
  deepEqual(step.effects, [{ kind: 'verify-staging', code: STAGING }]);
  equal(step.cue, undefined);
});

test('放货位：枪夹带的控制字符与首尾空白不带进去核', () => {
  const step = scanCode(startPick({ cells: [A], staging: null }), '\u0002 SYN-LOC-OUT-01\r\n');
  deepEqual(step.effects, [{ kind: 'verify-staging', code: STAGING }]);
});

test('放货位：核得过就定下，记在本机，扫对的声音，接着等库位', () => {
  const step = scanStaging(startPick({ cells: [A, B], staging: null }), STAGING, OK);
  equal(step.state.phase, 'location');
  equal(step.state.staging, STAGING);
  equal(step.state.verifying, null);
  equal(step.state.feedback.kind, 'ok');
  ok(step.state.feedback.text.includes(STAGING));
  deepEqual(step.effects, [{ kind: 'remember-staging', code: STAGING }]);
  equal(step.cue, 'ok');
});

test('放货位：核不过，红，写明是哪一条，什么都不记，仍停在等放货位', () => {
  const before = startPick({ cells: [A], staging: null });
  const step = scanStaging(before, 'SYN-LOC-STORE-01', NOT_STAGING_ZONE);
  equal(step.state.phase, 'staging');
  equal(step.state.staging, null);
  equal(step.state.verifying, null);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('没有记下'));
  ok(step.state.feedback.text.includes('用途是「存储」'));
  deepEqual(step.effects, []);
  equal(step.cue, 'wrong');
  equal(pickScreen(step.state).label, '扫错了');
  equal(pickScreen(step.state).headline, '这个库位不能当放货位');
});

test('放货位：没核成（读口没问成），不说成登记册里没有，说为什么，让人再扫一次，什么都不记', () => {
  const before = startPick({ cells: [A], staging: null });
  const step = scanStaging(before, STAGING, NO_NETWORK);
  equal(step.state.phase, 'staging');
  equal(step.state.staging, null);
  equal(step.state.verifying, null);
  equal(step.state.feedback.kind, 'unsent');
  equal(step.state.feedback.scanned, STAGING);
  const text = step.state.feedback.text;
  ok(text.includes('没能核对放货位 SYN-LOC-OUT-01'), text);
  ok(text.includes('网络没通（断了）'), text);
  ok(text.includes('没有记下') && text.includes('再扫一次'), text);
  ok(!text.includes('登记册里没有'), text);
  deepEqual(step.effects, []);
  equal(step.cue, 'wrong');
  // 没核成不是扫错：不摆「扫错了」，标题仍是等放货位
  equal(pickScreen(step.state).label, '下一步');
  equal(pickScreen(step.state).headline, '开工先扫待出库库位');
});

test('没核成的原因本身是带句号的整句时，拼进话里不出现「。。」', () => {
  const sentence = unverifiedBecause('读库位名册没成：所选设备号不在本租户设备册。');
  const scanned = scanStaging(startPick({ cells: [A], staging: null }), STAGING, sentence).state.feedback.text;
  ok(scanned.includes('所选设备号不在本租户设备册。没有记下') && !scanned.includes('。。'), scanned);
  const remembered = stagingVerified(beginPick([A], STAGING).state, STAGING, sentence).state.feedback.text;
  ok(remembered.includes('所选设备号不在本租户设备册。先不当它能用') && !remembered.includes('。。'), remembered);
});

test('放货位：没核成之后扫同一个再核，核得过就定下', () => {
  const failed = scanStaging(startPick({ cells: [A], staging: null }), STAGING, NO_NETWORK).state;
  const again = scanStaging(failed, STAGING, OK);
  equal(again.state.phase, 'location');
  equal(again.state.staging, STAGING);
  deepEqual(again.effects, [{ kind: 'remember-staging', code: STAGING }]);
});

test('放货位：核不过之后换一个核得过的，照常定下', () => {
  const bad = scanStaging(startPick({ cells: [A], staging: null }), 'SYN-LOC-STORE-01', NOT_STAGING_ZONE).state;
  const good = scanStaging(bad, 'SYN-LOC-OUT-02', OK);
  equal(good.state.phase, 'location');
  equal(good.state.staging, 'SYN-LOC-OUT-02');
});

test('核放货位的答复过期了（码对不上，或已不在核对中）就不动', () => {
  const verifying = scanCode(startPick({ cells: [A], staging: null }), STAGING).state;
  unchanged(verifying, stagingVerified(verifying, 'ANOTHER-CODE', OK));
  const atStaging = startPick({ cells: [A], staging: null });
  unchanged(atStaging, stagingVerified(atStaging, STAGING, OK));
  const location = atLocation();
  unchanged(location, stagingVerified(location, STAGING, OK));
  // 答复只落一次：落下之后同一个答复再来，不再改什么
  const placed = stagingVerified(verifying, STAGING, OK).state;
  unchanged(placed, stagingVerified(placed, STAGING, OK));
});

test('核对中不收扫码：连着来的扫码都不生效，状态不动', () => {
  const verifying = scanCode(startPick({ cells: [A], staging: null }), STAGING).state;
  equal(acceptingScans(verifying), false);
  for (const code of ['SYN-LOC-OUT-02', STAGING, 'A-01-03', 'SYN-SKU-EARBUD-01', '6900001000011']) {
    unchanged(verifying, scanCode(verifying, code));
  }
});

test('核对中的按钮都点不了，扫描框里的字说明为什么；刷新读到的队列也不换', () => {
  const verifying = scanCode(startPick({ cells: [A, B], staging: null }), STAGING).state;
  deepEqual(pickActions(verifying), {
    changeCount: false,
    shortPick: false,
    skip: false,
    changeStaging: false,
    refresh: false,
    resend: false,
    next: false,
    skipTote: false,
  });
  equal(scanPlaceholder(verifying), '正在处理，先不收扫码');
  unchanged(verifying, reloadQueue(verifying, [C]));
  unchanged(verifying, changeStaging(verifying));
  unchanged(verifying, skipCell(verifying));
  unchanged(verifying, dismissWrong(verifying));
  equal(changeCount(verifying, '3').ok, false);
});

test('核对中的屏上：请稍等，说在核哪个库位', () => {
  const screen = pickScreen(scanCode(startPick({ cells: [A], staging: null }), STAGING).state);
  equal(screen.label, '请稍等');
  equal(screen.headline, '正在核对放货位…');
  ok(screen.counter.includes(STAGING));
});

test('Esc 也清掉没核成的那句，回到等放货位', () => {
  const failed = scanStaging(startPick({ cells: [A], staging: null }), STAGING, NO_NETWORK).state;
  const cleared = dismissWrong(failed);
  equal(cleared.state.feedback.kind, 'waiting');
  equal(cleared.state.phase, 'staging');
  deepEqual(cleared.effects, []);
});

test('放货位：队里没有格时定下之后是做完了', () => {
  const step = scanStaging(startPick({ cells: [], staging: null }), STAGING, OK);
  equal(step.state.phase, 'finished');
});

// —— 本机记着的放货位：进来时按同一套核 ——

test('开一趟：本机没有记着放货位，就等人扫一个', () => {
  const step = beginPick([A, B], null);
  equal(step.state.phase, 'staging');
  equal(step.state.verifying, null);
  deepEqual(step.effects, []);
  deepEqual(step.state, startPick({ cells: [A, B], staging: null }));
});

test('开一趟：本机记着放货位，先核它，核的时候不收扫码，也不响', () => {
  const step = beginPick([A, B], STAGING);
  equal(step.state.phase, 'verifying');
  equal(step.state.staging, null);
  deepEqual(step.state.verifying, { code: STAGING, remembered: true });
  equal(step.state.feedback.kind, 'waiting');
  ok(step.state.feedback.text.includes(STAGING));
  equal(step.state.feedback.scanned, undefined);
  deepEqual(step.effects, [{ kind: 'verify-staging', code: STAGING }]);
  equal(step.cue, undefined);
  equal(acceptingScans(step.state), false);
  unchanged(step.state, scanCode(step.state, 'SYN-LOC-OUT-02'));
});

test('记着的放货位核得过：直接去等库位，不响，也不再往本机写一遍', () => {
  const verifying = beginPick([A, B], STAGING).state;
  const step = stagingVerified(verifying, STAGING, OK);
  equal(step.state.phase, 'location');
  equal(step.state.staging, STAGING);
  equal(step.state.verifying, null);
  equal(step.state.feedback.kind, 'waiting');
  deepEqual(step.effects, []);
  equal(step.cue, undefined);
  deepEqual(step.state, startPick({ cells: [A, B], staging: STAGING }));
});

test('记着的放货位核得过而队里没有格：是做完了', () => {
  const step = stagingVerified(beginPick([], STAGING).state, STAGING, OK);
  equal(step.state.phase, 'finished');
  equal(step.state.staging, STAGING);
});

test('记着的放货位现在核不过：不用它，屏上说是哪一条、让人重扫一个', () => {
  const verifying = beginPick([A], 'SYN-LOC-OLD').state;
  const step = stagingVerified(verifying, 'SYN-LOC-OLD', rejectedBecause('库位 SYN-LOC-OLD 已停用，不能放货。'));
  equal(step.state.phase, 'staging');
  equal(step.state.staging, null);
  equal(step.state.verifying, null);
  equal(step.state.feedback.kind, 'wrong');
  const text = step.state.feedback.text;
  ok(text.includes('上次用的放货位 SYN-LOC-OLD 现在不能用') && text.includes('已停用') && text.includes('重新扫'), text);
  deepEqual(step.effects, []);
  equal(step.cue, undefined);
});

test('记着的放货位这一次没核成：不静静当它合格，也不静静扔掉，说明并让人重扫（扫同一个即可）', () => {
  const verifying = beginPick([A], STAGING).state;
  const step = stagingVerified(verifying, STAGING, NO_NETWORK);
  equal(step.state.phase, 'staging');
  equal(step.state.staging, null);
  equal(step.state.feedback.kind, 'unsent');
  const text = step.state.feedback.text;
  ok(text.includes('上次用的放货位 SYN-LOC-OUT-01 这一次没核成') && text.includes('网络没通（断了）'), text);
  ok(text.includes('扫同一个就行'), text);
  ok(!text.includes('登记册里没有'), text);
  // 没有写本机，也没有删：不产生任何效果
  deepEqual(step.effects, []);
  // 重扫同一个，核得过就用它，并照扫到放货位那样记下
  const again = stagingVerified(scanCode(step.state, STAGING).state, STAGING, OK);
  equal(again.state.phase, 'location');
  equal(again.state.staging, STAGING);
  deepEqual(again.effects, [{ kind: 'remember-staging', code: STAGING }]);
});

test('记着的放货位的答复过期了（码对不上）就不动', () => {
  const verifying = beginPick([A], STAGING).state;
  unchanged(verifying, stagingVerified(verifying, 'SYN-LOC-OUT-02', OK));
});

// —— 换放货位 ——

test('换放货位：核得过之后回到原来那一步，已扫的数还在', () => {
  const mid = counted(2);
  const changing = changeStaging(mid).state;
  equal(changing.phase, 'staging');
  equal(changing.resume, 'goods');
  equal(pickScreen(changing).headline, '扫新的待出库库位');

  const scanning = scanCode(changing, 'SYN-LOC-OUT-02');
  equal(scanning.state.phase, 'verifying');
  equal(scanning.state.resume, 'goods');
  // 核的时候放货位仍是原来的，新的还没定下
  equal(scanning.state.staging, STAGING);
  equal(scanning.state.counted, 2);

  const back = stagingVerified(scanning.state, 'SYN-LOC-OUT-02', OK);
  equal(back.state.phase, 'goods');
  equal(back.state.counted, 2);
  equal(back.state.staging, 'SYN-LOC-OUT-02');
  equal(back.state.resume, null);
  deepEqual(back.effects, [{ kind: 'remember-staging', code: 'SYN-LOC-OUT-02' }]);
});

test('换放货位：核不过或没核成，仍留在等放货位、仍记得原来是在哪一步，原来的放货位不变', () => {
  const changing = changeStaging(counted(2)).state;
  const rejected = scanStaging(changing, 'SYN-LOC-OLD', rejectedBecause('库位 SYN-LOC-OLD 已停用，不能放货。'));
  equal(rejected.state.phase, 'staging');
  equal(rejected.state.resume, 'goods');
  equal(rejected.state.staging, STAGING);
  deepEqual(rejected.effects, []);

  const unverified = scanStaging(changing, 'SYN-LOC-OUT-02', NO_NETWORK);
  equal(unverified.state.phase, 'staging');
  equal(unverified.state.resume, 'goods');
  equal(unverified.state.staging, STAGING);
  deepEqual(unverified.effects, []);

  // 没核成之后重扫同一个：核得过才换，并回到原来那一步
  const retried = scanStaging(unverified.state, 'SYN-LOC-OUT-02', OK);
  equal(retried.state.phase, 'goods');
  equal(retried.state.counted, 2);
  equal(retried.state.staging, 'SYN-LOC-OUT-02');
});

test('换放货位：从没成的格或做完那一步换，核得过之后也回到那里', () => {
  const refused = applyVerdict(submittingState(), judgeWorkFactAnswer({ kind: 'notGranted' }, pickOutcomeLabels)).state;
  const fromRefused = scanStaging(changeStaging(refused).state, 'SYN-LOC-OUT-02', OK);
  equal(fromRefused.state.phase, 'refused');
  equal(fromRefused.state.staging, 'SYN-LOC-OUT-02');
  const finished = applyVerdict(submittingState([C]), done).state;
  const fromFinished = scanStaging(changeStaging(finished).state, 'SYN-LOC-OUT-02', OK);
  equal(fromFinished.state.phase, 'finished');
});

test('换放货位：提交、认码、核对、没送到时不让换', () => {
  const submitting = scanCode(counted(4), A.goodsCode).state;
  equal(submitting.phase, 'submitting');
  unchanged(submitting, changeStaging(submitting));
  const resolving = scanCode(atGoods(), '6900001000011').state;
  equal(resolving.phase, 'resolving');
  unchanged(resolving, changeStaging(resolving));
  const verifying = scanCode(changeStaging(counted(1)).state, 'SYN-LOC-OUT-02').state;
  equal(verifying.phase, 'verifying');
  unchanged(verifying, changeStaging(verifying));
});

// —— 等库位 ——

test('扫对库位：进到等商品，计数从 0 数起', () => {
  const step = scanCode(atLocation(), 'A-01-03');
  equal(step.state.phase, 'goods');
  equal(step.state.counted, 0);
  equal(step.state.feedback.kind, 'ok');
  equal(step.cue, 'ok');
  deepEqual(step.effects, []);
});

test('库位码规范化后才比：枪夹带的控制字符与首尾空白不算错', () => {
  equal(scanCode(atLocation(), '\u0002 A-01-03\r\n').state.phase, 'goods');
});

test('扫错库位：红，说扫到的是什么、这一格在哪，什么都不记', () => {
  const before = atLocation();
  const step = scanCode(before, 'A-02-01');
  equal(step.state.phase, 'location');
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('A-02-01'));
  ok(step.state.feedback.text.includes('A-01-03'));
  equal(step.state.counted, 0);
  deepEqual(step.state.queue, before.queue);
  deepEqual(step.effects, []);
  equal(step.cue, 'wrong');
  equal(pickScreen(step.state).label, '扫错了');
  equal(pickScreen(step.state).headline, '这不是这一格的库位');
});

test('等库位时扫到商品码或不相干的码，也是扫错', () => {
  for (const code of ['SYN-SKU-EARBUD-01', '6900001000011', 'SYN-LOC-OUT-01', 'whatever']) {
    const step = scanCode(atLocation(), code);
    equal(step.state.phase, 'location', code);
    equal(step.state.feedback.kind, 'wrong', code);
    equal(step.cue, 'wrong', code);
  }
});

test('空码不算扫码：状态一点不动，不响', () => {
  for (const state of [atLocation(), atGoods(), startPick({ cells: [A], staging: null })]) {
    unchanged(state, scanCode(state, ''));
    unchanged(state, scanCode(state, '  \r\n'));
  }
});

test('再扫对的就恢复：扫错之后扫对库位照常往下走', () => {
  const wrongStep = scanCode(atLocation(), 'A-02-01');
  const recovered = scanCode(wrongStep.state, 'A-01-03');
  equal(recovered.state.phase, 'goods');
  equal(recovered.state.feedback.kind, 'ok');
});

// —— 等商品并计数 ——

test('扫到商品编码：算对，加一，什么都不用问后端', () => {
  const step = scanCode(atGoods(), 'SYN-SKU-EARBUD-01');
  equal(step.state.phase, 'goods');
  equal(step.state.counted, 1);
  equal(step.state.feedback.kind, 'ok');
  equal(step.cue, 'ok');
  deepEqual(step.effects, []);
});

test('逐件加一，屏上的字跟着变：还差几件、已扫几件', () => {
  const state = counted(2);
  equal(pickScreen(state).headline, '扫商品 SYN-SKU-EARBUD-01，还差 3 件');
  equal(pickScreen(state).counter, '已扫 2 / 5');
  equal(pickScreen(state).label, '下一步');
});

test('扫满应拣数自动提交这一格：按应拣数，带上这一格', () => {
  const step = scanCode(counted(4), 'SYN-SKU-EARBUD-01');
  equal(step.state.phase, 'submitting');
  equal(step.state.counted, 5);
  deepEqual(step.state.submission, { cell: A, quantity: 5 });
  deepEqual(step.effects, [{ kind: 'submit', cell: A, quantity: 5 }]);
  equal(step.cue, 'ok');
  ok(step.state.feedback.text.includes('正在提交'));
});

test('商品这一格扫到别的东西：不记数，红，说明这一格还差几件', () => {
  const state = counted(2);
  const step = scanCode(state, 'SYN-LOC-OUT-01');
  equal(step.state.counted, 2);
  equal(step.state.phase, 'goods');
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('还差 3 件'));
  equal(step.cue, 'wrong');
});

test('等商品时又扫了一次库位码：算扫错，说明是库位码、这一格还差几件', () => {
  const step = scanCode(counted(1), 'A-01-03');
  equal(step.state.counted, 1);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('库位码'));
  ok(step.state.feedback.text.includes('还差 4 件'));
  deepEqual(step.effects, []);
  equal(step.cue, 'wrong');
});

test('扫到别的码要去认：进认码中，带着这一格与这个码；认的时候不收别的扫码', () => {
  const step = scanCode(counted(2), '6900001000011');
  equal(step.state.phase, 'resolving');
  equal(step.state.counted, 2);
  deepEqual(step.effects, [{ kind: 'identify', cell: A, code: '6900001000011' }]);
  equal(step.cue, undefined);
  unchanged(step.state, scanCode(step.state, 'SYN-SKU-EARBUD-01'));
});

test('认成这一格的商品（条码）：算对，加一', () => {
  const resolving = scanCode(counted(2), '6900001000011').state;
  const step = goodsIdentified(resolving, '6900001000011', { kind: 'same', via: 'alias' });
  equal(step.state.phase, 'goods');
  equal(step.state.counted, 3);
  equal(step.state.feedback.kind, 'ok');
  ok(step.state.feedback.text.includes('这个码是 SYN-SKU-EARBUD-01'));
  equal(step.cue, 'ok');
});

test('认成条码而扫满了：照样自动提交', () => {
  const resolving = scanCode(counted(4), '6900001000011').state;
  const step = goodsIdentified(resolving, '6900001000011', { kind: 'same', via: 'alias' });
  equal(step.state.phase, 'submitting');
  deepEqual(step.effects, [{ kind: 'submit', cell: A, quantity: 5 }]);
});

test('认成别的商品：算错，不记数，说认成了哪个、这一格要哪个、还差几件', () => {
  const resolving = scanCode(counted(2), '6900001000028').state;
  const step = goodsIdentified(resolving, '6900001000028', { kind: 'other', goodsCode: 'SYN-SKU-CHARGER-02' });
  equal(step.state.phase, 'goods');
  equal(step.state.counted, 2);
  equal(step.state.feedback.kind, 'wrong');
  const text = step.state.feedback.text;
  ok(text.includes('6900001000028') && text.includes('SYN-SKU-CHARGER-02') && text.includes('SYN-SKU-EARBUD-01') && text.includes('还差 3 件'));
  equal(step.cue, 'wrong');
  equal(pickScreen(step.state).headline, '这不是这一格要的货');
});

test('认不出：算错，不记数', () => {
  const resolving = scanCode(counted(1), '6900009999999').state;
  const step = goodsIdentified(resolving, '6900009999999', { kind: 'unknown' });
  equal(step.state.counted, 1);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('认不出'));
  equal(step.cue, 'wrong');
});

test('没能认（读口没问成）：算错，不记数，写明原因，让人再扫一次', () => {
  const resolving = scanCode(counted(1), '6900001000028').state;
  const step = goodsIdentified(resolving, '6900001000028', { kind: 'failed', reason: '读商品资料没成：网络没通（断了）' });
  equal(step.state.counted, 1);
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('网络没通'));
  ok(step.state.feedback.text.includes('再扫一次'));
});

test('认码的答复过期了（码对不上，或已不在认码中）就不动', () => {
  const resolving = scanCode(counted(1), '6900001000028').state;
  unchanged(resolving, goodsIdentified(resolving, 'ANOTHER-CODE', { kind: 'same', via: 'alias' }));
  const goods = counted(1);
  unchanged(goods, goodsIdentified(goods, '6900001000028', { kind: 'same', via: 'alias' }));
});

test('Esc 清掉扫错，回到等扫；没有扫错时不动', () => {
  const wrongState = scanCode(counted(2), 'SYN-LOC-OUT-01').state;
  equal(wrongState.feedback.kind, 'wrong');
  const cleared = dismissWrong(wrongState);
  equal(cleared.state.feedback.kind, 'waiting');
  equal(cleared.state.counted, 2);
  equal(cleared.state.phase, 'goods');
  const fine = counted(1);
  unchanged(fine, dismissWrong(fine));
});

test('Esc 只在收扫码的几步里清扫错', () => {
  const failed = applyVerdict(scanCode(counted(4), A.goodsCode).state, judgeWorkFactAnswer({ kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' }, pickOutcomeLabels)).state;
  equal(failed.phase, 'refused');
  equal(failed.feedback.kind, 'wrong');
  unchanged(failed, dismissWrong(failed));
});

test('扫错之后再扫对的就恢复', () => {
  const wrongState = scanCode(counted(2), 'SYN-LOC-OUT-01').state;
  const recovered = scanCode(wrongState, 'SYN-SKU-EARBUD-01');
  equal(recovered.state.counted, 3);
  equal(recovered.state.feedback.kind, 'ok');
});

// —— 改数量 ——

test('数量的判定：1 到应拣之间的整数', () => {
  deepEqual(judgeQuantity('1', 5), { ok: true, value: 1 });
  deepEqual(judgeQuantity('5', 5), { ok: true, value: 5 });
  deepEqual(judgeQuantity(' 3 ', 5), { ok: true, value: 3 });
  deepEqual(judgeQuantity('007', 9), { ok: true, value: 7 });
});

test('数量的边界：0、负数、超过应拣、小数、非数字、空，都不收，并说为什么', () => {
  for (const [text, reason] of [
    ['', '请填数量'],
    ['   ', '请填数量'],
    ['abc', '整数'],
    ['2.5', '整数'],
    ['-1', '整数'],
    ['1e2', '整数'],
    ['+3', '整数'],
    ['0', '至少 1 件'],
    ['00', '至少 1 件'],
    ['6', '最多 5 件'],
    ['999999', '最多 5 件'],
  ] as const) {
    const judged = judgeQuantity(text, 5);
    equal(judged.ok, false, text);
    if (!judged.ok) ok(judged.reason.includes(reason), `${text}: ${judged.reason}`);
  }
});

test('改数量：直接把已扫数改成指定的数，不用一件件扫', () => {
  const result = changeCount(atGoods(), '3');
  ok(result.ok);
  if (!result.ok) return;
  equal(result.step.state.counted, 3);
  equal(result.step.state.phase, 'goods');
  equal(result.step.state.feedback.kind, 'ok');
  deepEqual(result.step.effects, []);
});

test('改数量可以往下改：扫多了改回去', () => {
  const result = changeCount(counted(4), '2');
  ok(result.ok);
  if (result.ok) equal(result.step.state.counted, 2);
});

test('改数量改到应拣数就像扫满一样自动提交', () => {
  const result = changeCount(atGoods(), '5');
  ok(result.ok);
  if (!result.ok) return;
  equal(result.step.state.phase, 'submitting');
  deepEqual(result.step.effects, [{ kind: 'submit', cell: A, quantity: 5 }]);
});

test('改数量填得不对：不改，说明为什么', () => {
  for (const text of ['0', '6', 'x', '']) {
    const result = changeCount(counted(2), text);
    equal(result.ok, false, text);
  }
});

test('已经扫了数的格改成 0：不能退回去，指给「少拣」；一件没扫时指给「跳过这一格」', () => {
  const scanned = changeCount(counted(2), '0');
  equal(scanned.ok, false);
  if (!scanned.ok) ok(scanned.reason.includes('已经扫了 2 件') && scanned.reason.includes('少拣'), scanned.reason);

  const none = changeCount(atGoods(), '0');
  equal(none.ok, false);
  if (!none.ok) ok(none.reason.includes('跳过这一格'), none.reason);
});

test('改数量只在等商品时可以：还没扫库位、提交中都不行', () => {
  equal(changeCount(atLocation(), '3').ok, false);
  const submitting = scanCode(counted(4), A.goodsCode).state;
  equal(changeCount(submitting, '3').ok, false);
});

// —— 少拣 ——

test('少拣的前提：在等商品，已扫的数在 1 与应拣之间；一件都没扫、已够数都不行', () => {
  equal(shortPickPlan(atGoods()), null);
  const plan = shortPickPlan(counted(3));
  deepEqual(plan, { cell: A, quantity: 3, short: 2, staging: STAGING });
  equal(shortPickPlan({ ...counted(1), counted: 5 }), null);
  equal(shortPickPlan(atLocation()), null);
});

test('少拣：按当前已扫的数提交，这一格带着实拣数量', () => {
  const step = submitShortPick(counted(3));
  equal(step.state.phase, 'submitting');
  deepEqual(step.state.submission, { cell: A, quantity: 3 });
  deepEqual(step.effects, [{ kind: 'submit', cell: A, quantity: 3 }]);
});

test('一件都没扫的少拣提交不出去：0 件后端不收', () => {
  const before = atGoods();
  unchanged(before, submitShortPick(before));
});

test('少拣确认框里的话照核到的规则：记成短拣、不能再拣、差额记一笔、差的换别的库位补、换不出才回到已预占', () => {
  const plan = shortPickPlan(counted(3));
  ok(plan !== null);
  if (plan === null) return;
  const text = shortPickLines(plan).join('');
  for (const must of ['要拣 5 件', '只记 3 件', STAGING, '差的 2 件', '短拣', '不能再拣', '换别的库位补', '添进余下的格', '已预占']) {
    ok(text.includes(must), must);
  }
});

test('少拣成功：这一格出队，说明按几件记成了短拣', () => {
  const submitting = submitShortPick(counted(3)).state;
  const step = applyVerdict(submitting, judgeWorkFactAnswer({ kind: 'outcome', status: 201, body: { outcome: 'PICKED' } }, pickOutcomeLabels));
  ok(step.state.feedback.text.includes('短拣'));
  ok(step.state.feedback.text.includes('3 件'));
  equal(step.state.done, 1);
  deepEqual(step.state.queue, [B, C]);
});

// —— 跳过 ——

test('跳过这一格：挪到队尾，不提交', () => {
  const step = skipCell(atLocation());
  deepEqual(step.state.queue, [B, C, A]);
  equal(step.state.phase, 'location');
  equal(step.state.done, 0);
  deepEqual(step.effects, []);
  ok(step.state.feedback.text.includes('A-01-03'));
});

test('连着跳：次序一格格往后轮', () => {
  const once = skipCell(atLocation()).state;
  const twice = skipCell(once).state;
  deepEqual(twice.queue, [C, A, B]);
  equal(pickScreen(twice).headline, '去库位 B-01-02，扫库位码');
});

test('跳过之后做完下一格，队首是排在前面的，被跳过的在最后', () => {
  const skipped = skipCell(atLocation()).state;
  // 现在队首是 B：扫库位、扫够 3 件、提交成功
  let state = scanCode(skipped, 'A-01-05').state;
  for (let i = 0; i < 3; i += 1) state = scanCode(state, 'SYN-SKU-CHARGER-02').state;
  equal(state.phase, 'submitting');
  const done = applyVerdict(state, judgeWorkFactAnswer({ kind: 'outcome', status: 201, body: { outcome: 'PICKED' } }, pickOutcomeLabels)).state;
  deepEqual(done.queue, [C, A]);
  equal(done.done, 1);
});

test('已经在等商品但还一件没扫，也可以跳过；扫了数就不让跳（那几件在手里）', () => {
  equal(skipCell(atGoods()).state.queue[0].taskId, 'T-B');
  const mid = counted(1);
  unchanged(mid, skipCell(mid));
});

test('队里只有这一格时没处可挪，不让跳', () => {
  const only = atLocation([A]);
  unchanged(only, skipCell(only));
  equal(pickActions(only).skip, false);
});

// —— 提交的报文：与现有作业台 pick 那条定义装出来的逐键相同 ——

test('拣货的报文与旧作业台页用同一条 pick 定义装出来的逐键相同（后端用封闭载荷解码，多一个键、错一个字就是 400）', () => {
  const spec = workFactSpecs.find((candidate) => candidate.id === 'pick');
  ok(spec !== undefined);
  if (spec === undefined) return;
  const at = new Date('2026-11-25T08:30:15.123Z');
  const device = 'SYN-PDA-WH01-01';
  for (const submission of [
    { cell: A, quantity: 5 },
    { cell: A, quantity: 3 },
    { cell: C, quantity: 1 },
  ]) {
    const prepared = prepareWorkFact({
      spec,
      deviceId: device,
      warehouse: 'SYN-WH-01',
      sequence: 9,
      values: pickFactValues(submission, STAGING),
      now: at,
    });
    ok(prepared.ok);
    if (!prepared.ok) return;
    // 旧作业台页的做法：事实号与仓按定义补，和人在表单里填的格一起过 buildGuidedPayload，再叠固定载荷
    const old = buildGuidedPayload(spec.form, {
      ...workFactPrefillFor(spec, device, 'SYN-WH-01', 9, at),
      taskId: submission.cell.taskId,
      quantity: String(submission.quantity),
      targetLocation: STAGING,
    });
    ok(old.ok);
    if (!old.ok) return;
    deepEqual(prepared.attempt.payload, { ...(spec.fixedPayload ?? {}), ...old.payload });
    deepEqual(prepared.attempt.payload, {
      factId: 'PDA-SYN-PDA-WH01-01-20261125083015-9',
      warehouse: 'SYN-WH-01',
      taskId: submission.cell.taskId,
      quantity: submission.quantity,
      targetLocation: STAGING,
    });
  }
});

test('拣货要填的格：任务号、实拣数量（文本）、放货位', () => {
  deepEqual(pickFactValues({ cell: A, quantity: 3 }, STAGING), { taskId: 'T-A', quantity: '3', targetLocation: STAGING });
});

// —— 提交的三种结局 ——

const done = judgeWorkFactAnswer({ kind: 'outcome', status: 201, body: { outcome: 'PICKED' } }, pickOutcomeLabels);

function submittingState(cells: PickCell[] = [A, B, C]): PickState {
  let state = atGoods(cells);
  const quantity = cells[0].quantity;
  for (let i = 0; i < quantity; i += 1) state = scanCode(state, cells[0].goodsCode).state;
  return state;
}

test('提交成了：绿，这一格出队，进下一格的等库位', () => {
  const submitting = submittingState();
  const step = applyVerdict(submitting, done);
  equal(step.state.phase, 'location');
  deepEqual(step.state.queue, [B, C]);
  equal(step.state.done, 1);
  equal(step.state.counted, 0);
  equal(step.state.submission, null);
  equal(step.state.feedback.kind, 'ok');
  ok(step.state.feedback.text.includes('5 件'));
  ok(step.state.feedback.text.includes(STAGING));
  equal(step.cue, 'ok');
  equal(pickScreen(step.state).headline, '去库位 A-01-05，扫库位码');
});

test('提交成了而这是最后一格：全部做完', () => {
  const step = applyVerdict(submittingState([C]), done);
  equal(step.state.phase, 'finished');
  equal(step.state.queue.length, 0);
  equal(pickScreen(step.state).headline, '拣完了');
  equal(pickScreen(step.state).counter, '这一趟拣了 1 格');
  equal(pickProgress(step.state), '已拣完，共 1 格');
});

test('已有结果也算成功，说明是以前记过的、这次没有重复记', () => {
  const verdict = judgeWorkFactAnswer({ kind: 'outcome', status: 200, body: { outcome: 'EXISTING_RESULT' } }, pickOutcomeLabels);
  equal(verdict.verdict, 'done');
  const step = applyVerdict(submittingState(), verdict);
  equal(step.state.phase, 'location');
  equal(step.state.done, 1);
  ok(step.state.feedback.text.includes('没有重复记'));
});

test('出库单已截单而照记的那一笔：算成功，说明货已生成回库任务', () => {
  const verdict = judgeWorkFactAnswer({ kind: 'outcome', status: 201, body: { outcome: 'RECORDED' } }, pickOutcomeLabels);
  const step = applyVerdict(submittingState(), verdict);
  equal(step.state.done, 1);
  ok(step.state.feedback.text.includes('截单'));
  ok(step.state.feedback.text.includes('回库'));
});

test('没送到：停在没送到，保留这一格与已扫的数，等「再送一次」；不出队、不记进度', () => {
  const submitting = submittingState();
  const step = applyVerdict(submitting, judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, pickOutcomeLabels));
  equal(step.state.phase, 'unsent');
  equal(step.state.feedback.kind, 'unsent');
  ok(step.state.feedback.text.includes('再送一次'));
  deepEqual(step.state.queue, [A, B, C]);
  equal(step.state.done, 0);
  equal(step.state.counted, 5);
  deepEqual(step.state.submission, { cell: A, quantity: 5 });
  equal(step.cue, 'wrong');
  equal(pickActions(step.state).resend, true);
});

test('后端没能处理（5xx）同样可以再送', () => {
  const step = applyVerdict(submittingState(), judgeWorkFactAnswer({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }, pickOutcomeLabels));
  equal(step.state.phase, 'unsent');
});

test('再送一次：回到提交中，效果是重送（用同一个事实号，由接线保留那一笔）', () => {
  const unsent = applyVerdict(submittingState(), judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, pickOutcomeLabels)).state;
  const step = resend(unsent);
  equal(step.state.phase, 'submitting');
  deepEqual(step.effects, [{ kind: 'resend' }]);
  deepEqual(step.state.submission, unsent.submission);
  // 再送成功就是正常的成功
  const finished = applyVerdict(step.state, judgeWorkFactAnswer({ kind: 'outcome', status: 200, body: { outcome: 'EXISTING_RESULT' } }, pickOutcomeLabels));
  equal(finished.state.done, 1);
});

test('只有没送到时才能再送', () => {
  const location = atLocation();
  unchanged(location, resend(location));
  const submitting = submittingState();
  unchanged(submitting, resend(submitting));
});

test('业务上没成：红，说明原因，不出队，给「下一格」', () => {
  const verdict = judgeWorkFactAnswer({ kind: 'outcome', status: 200, body: { outcome: 'CONTENT_INVALID' } }, pickOutcomeLabels);
  const step = applyVerdict(submittingState(), verdict);
  equal(step.state.phase, 'refused');
  equal(step.state.feedback.kind, 'wrong');
  ok(step.state.feedback.text.includes('放货位'));
  deepEqual(step.state.queue, [A, B, C]);
  equal(step.state.done, 0);
  equal(step.cue, 'wrong');
  equal(pickActions(step.state).next, true);
  equal(pickScreen(step.state).headline, '这一格没有记成');
});

test('没成而任务还能拣（内容不合）：「下一格」把它挪到队尾，回头再来', () => {
  const refused = applyVerdict(
    submittingState(),
    judgeWorkFactAnswer({ kind: 'outcome', status: 200, body: { outcome: 'CONTENT_INVALID' } }, pickOutcomeLabels),
  ).state;
  equal(refused.gone, false);
  const step = nextCell(refused);
  equal(step.state.phase, 'location');
  deepEqual(step.state.queue, [B, C, A]);
  equal(step.state.total, 3);
  equal(step.state.counted, 0);
  equal(step.state.submission, null);
});

test('没成而任务已经不能拣（已拣过、找不到）：「下一格」把它去掉，总数跟着减', () => {
  for (const outcome of ['CONTENT_CONFLICT', 'WAVE_NOT_FOUND']) {
    const refused = applyVerdict(
      submittingState(),
      judgeWorkFactAnswer({ kind: 'outcome', status: 200, body: { outcome } }, pickOutcomeLabels),
    ).state;
    equal(refused.gone, true, outcome);
    const step = nextCell(refused);
    deepEqual(step.state.queue, [B, C], outcome);
    equal(step.state.total, 2, outcome);
    equal(step.state.phase, 'location', outcome);
  }
  equal(taskGoneAfter('CONTENT_CONFLICT'), true);
  equal(taskGoneAfter('CONTENT_INVALID'), false);
  equal(taskGoneAfter(undefined), false);
});

test('去掉的是最后一格就是做完了', () => {
  const refused = applyVerdict(
    submittingState([C]),
    judgeWorkFactAnswer({ kind: 'outcome', status: 200, body: { outcome: 'CONTENT_CONFLICT' } }, pickOutcomeLabels),
  ).state;
  const step = nextCell(refused);
  equal(step.state.phase, 'finished');
});

test('接入被拒、问题码：没成，别再送，只给「下一格」', () => {
  for (const answer of [
    { kind: 'notGranted' } as const,
    { kind: 'loginRequired' } as const,
    { kind: 'unconfigured' } as const,
    { kind: 'callerProblem', status: 403, code: 'DEVICE_NOT_IN_USE' } as const,
  ]) {
    const step = applyVerdict(submittingState(), judgeWorkFactAnswer(answer, pickOutcomeLabels));
    equal(step.state.phase, 'refused', answer.kind);
    equal(step.state.gone, false, answer.kind);
    equal(pickActions(step.state).resend, false, answer.kind);
  }
});

test('结果只在提交中才收：别的阶段来了答复（过期的）不动', () => {
  const location = atLocation();
  unchanged(location, applyVerdict(location, done));
  const refused = applyVerdict(submittingState(), judgeWorkFactAnswer({ kind: 'notGranted' }, pickOutcomeLabels)).state;
  unchanged(refused, applyVerdict(refused, done));
});

test('核对放货位中、提交中、没送到、没记成、做完的阶段不收扫码：状态不动', () => {
  const verifying = scanCode(startPick({ cells: [A], staging: null }), STAGING).state;
  const submitting = submittingState();
  const unsent = applyVerdict(submitting, judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, pickOutcomeLabels)).state;
  const refused = applyVerdict(submitting, judgeWorkFactAnswer({ kind: 'notGranted' }, pickOutcomeLabels)).state;
  const finished = applyVerdict(submittingState([C]), done).state;
  for (const state of [verifying, submitting, unsent, refused, finished]) {
    unchanged(state, scanCode(state, 'A-01-05'));
    unchanged(state, scanCode(state, 'SYN-SKU-EARBUD-01'));
    equal(acceptingScans(state), false, state.phase);
  }
});

test('只有等放货位、等库位、等商品这三步收扫码', () => {
  equal(acceptingScans(startPick({ cells: [A], staging: null })), true);
  equal(acceptingScans(atLocation()), true);
  equal(acceptingScans(atGoods()), true);
  equal(acceptingScans(scanCode(counted(1), '6900001000011').state), false);
  equal(acceptingScans(scanCode(startPick({ cells: [A], staging: null }), STAGING).state), false);
});

// —— 做完与刷新 ——

test('做完之后：刷新读到新的格就接着做；读到零格仍说没有', () => {
  const finished = applyVerdict(submittingState([C]), done).state;
  const reloaded = reloadQueue(finished, [A, B]);
  equal(reloaded.state.phase, 'location');
  equal(reloaded.state.total, 2);
  equal(reloaded.state.done, 0);
  deepEqual(reloaded.state.queue, [A, B]);
  equal(reloadQueue(finished, []).state.phase, 'finished');
});

test('刷新：扫了一半的、提交中的、没送到的不让刷，免得手里的货没了去处', () => {
  const mid = counted(2);
  unchanged(mid, reloadQueue(mid, [B]));
  equal(pickActions(mid).refresh, false);
  const submitting = submittingState();
  unchanged(submitting, reloadQueue(submitting, [B]));
  const unsent = applyVerdict(submitting, judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, pickOutcomeLabels)).state;
  unchanged(unsent, reloadQueue(unsent, [B]));
  equal(pickActions(unsent).refresh, false);
});

test('没有放货位时刷新读到格，仍先等放货位', () => {
  const state = startPick({ cells: [], staging: null });
  equal(reloadQueue(state, [A]).state.phase, 'staging');
});

test('没有可做的格：说没有还没拣的格，而不是「拣完了」', () => {
  const state = startPick({ cells: [], staging: STAGING });
  equal(pickScreen(state).headline, '没有还没拣的格');
  equal(pickProgress(state), '没有待拣的格');
});

// —— 屏上的字 ——

test('屏上的字照屏幕示意：等库位', () => {
  const screen = pickScreen(atLocation());
  equal(screen.label, '下一步');
  equal(screen.headline, '去库位 A-01-03，扫库位码');
  equal(screen.counter, '这一格要拣 SYN-SKU-EARBUD-01，共 5 件');
});

test('屏上的字：等放货位', () => {
  const screen = pickScreen(startPick({ cells: [A], staging: null }));
  equal(screen.headline, '开工先扫待出库库位');
  ok(screen.counter.includes('都放到那里'));
});

test('进度：第几格，共几格；跳过的不算做完，做完一格进度加一', () => {
  equal(pickProgress(atLocation()), '第 1 格，共 3 格');
  equal(pickProgress(skipCell(atLocation()).state), '第 1 格，共 3 格');
  const afterOne = applyVerdict(submittingState(), done).state;
  equal(pickProgress(afterOne), '第 2 格，共 3 格');
});

test('屏上的标题与小字里没有英文状态码', () => {
  const states = [
    startPick({ cells: [A], staging: null }),
    atLocation(),
    atGoods(),
    scanCode(atLocation(), 'x').state,
    scanCode(atGoods(), 'x').state,
    submittingState(),
    beginPick([A], STAGING).state,
    scanCode(startPick({ cells: [A], staging: null }), STAGING).state,
    scanStaging(startPick({ cells: [A], staging: null }), STAGING, NO_NETWORK).state,
    scanStaging(startPick({ cells: [A], staging: null }), 'SYN-LOC-STORE-01', NOT_STAGING_ZONE).state,
  ];
  for (const state of states) {
    const screen = pickScreen(state);
    for (const text of [screen.headline, screen.counter, screen.label]) {
      ok(!/OUTBOUND_STAGING|OPEN|SHORT|RESERVED|PICKED|CONTENT_/.test(text), text);
    }
  }
});

test('拣货工位对个别答复的说法：没有英文码，也不自相矛盾', () => {
  for (const [outcome, text] of Object.entries(pickOutcomeLabels)) {
    ok(!/[A-Z]{4,}/.test(text), `${outcome}: ${text}`);
  }
});

test('各按钮此刻能不能点', () => {
  const location = pickActions(atLocation());
  deepEqual(location, {
    changeCount: false,
    shortPick: false,
    skip: true,
    changeStaging: true,
    refresh: true,
    resend: false,
    next: false,
    skipTote: false,
  });
  const goods0 = pickActions(atGoods());
  equal(goods0.changeCount, true);
  equal(goods0.shortPick, false);
  equal(goods0.skip, true);
  equal(goods0.refresh, true);
  const goods2 = pickActions(counted(2));
  equal(goods2.shortPick, true);
  equal(goods2.skip, false);
  equal(goods2.refresh, false);
  const staging = pickActions(startPick({ cells: [A], staging: null }));
  equal(staging.changeCount, false);
  equal(staging.changeStaging, false);
});

test('不收码时扫描框里的字说明为什么', () => {
  equal(scanPlaceholder(atLocation()), '扫码，或敲进去按回车');
  equal(scanPlaceholder(submittingState()), '正在处理，先不收扫码');
  const unsent = applyVerdict(submittingState(), judgeWorkFactAnswer({ kind: 'transport', message: '断了' }, pickOutcomeLabels)).state;
  ok(scanPlaceholder(unsent).includes('再送一次'));
  const refused = applyVerdict(submittingState(), judgeWorkFactAnswer({ kind: 'notGranted' }, pickOutcomeLabels)).state;
  ok(scanPlaceholder(refused).includes('下一格'));
});

// —— 队列读得全不全 ——

test('队列读全了没有要说的；只看了前几页就如实说看了多少', () => {
  deepEqual(queueNotes({ wavesRead: 3, truncated: false, waveMissing: false }, null), []);
  const notes = queueNotes({ wavesRead: 1000, truncated: true, waveMissing: false }, null);
  equal(notes.length, 1);
  ok(notes[0].includes('只看了最近放出的 1000 个还没拣完的波次'));
});

test('指名的波次没找到：说没找到', () => {
  ok(queueNotes({ wavesRead: 0, truncated: false, waveMissing: true }, 'W-9')[0].includes('没有找到波次 W-9'));
});
