import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import { workFactOutcomeLabels, workFactSpecs, type WorkFactSpec } from '../work-fact';
import { advanceDeviceSequence, readDeviceSequence, type StorageLike } from './device-binding';
import {
  describeReadFailure,
  judgeWorkFactAnswer,
  mintWorkFact,
  outcomeRecorded,
  prepareWorkFact,
  sendWorkFact,
  type WorkFactAttempt,
  type WorkFactPoster,
} from './work-fact-submit';

function memory(initial: Record<string, string> = {}): StorageLike {
  const data = new Map(Object.entries(initial));
  return {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value);
    },
  };
}

const specOf = (id: string): WorkFactSpec => {
  const spec = workFactSpecs.find((candidate) => candidate.id === id);
  if (spec === undefined) throw new Error(`没有作业 ${id}`);
  return spec;
};

const at = new Date('2026-11-25T08:30:15.123Z');
const device = 'SYN-PDA-WH01-01';

function prepared(input: Parameters<typeof prepareWorkFact>[0]): WorkFactAttempt {
  const result = prepareWorkFact(input);
  if (!result.ok) throw new Error(result.reason);
  return result.attempt;
}

// —— 报文：与旧作业台页同一份做法，逐键相同 ——

test('拣货的报文：键与值与旧作业台页 pick 那条定义装出来的逐个相同', () => {
  const spec = specOf('pick');
  const attempt = prepared({
    spec,
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 8,
    values: { taskId: 'PKT-1', quantity: '5', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  });
  deepEqual(attempt.payload, {
    factId: 'PDA-SYN-PDA-WH01-01-20261125083015-8',
    warehouse: 'SYN-WH-01',
    taskId: 'PKT-1',
    quantity: 5,
    targetLocation: 'SYN-LOC-OUT-01',
  });
  equal(attempt.factId, 'PDA-SYN-PDA-WH01-01-20261125083015-8');
  equal(attempt.endpoint, '/warehouse-fulfillment-picks');
});

test('拣货的报文键与定义里的格一一对应：定义多一格或少一格，这里就红', () => {
  const spec = specOf('pick');
  const attempt = prepared({
    spec,
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 1,
    values: { taskId: 'PKT-1', quantity: '1', targetLocation: 'SYN-LOC-OUT-01', toteCode: 'SYN-TOTE-01' },
    now: at,
  });
  deepEqual(
    Object.keys(attempt.payload).sort(),
    spec.form.fields.map((field) => field.path).sort(),
  );
  // 箱号是可选的一格：没用箱时不带这一键，后端把不带读成没用箱，空串它不收
  const withoutTote = prepared({
    spec,
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 2,
    values: { taskId: 'PKT-1', quantity: '1', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  });
  equal('toteCode' in withoutTote.payload, false);
  // 拣货不带发生时刻与设备序号；后端用封闭载荷解码，多一个键就是 400
  equal('occurredAt' in attempt.payload, false);
  equal('deviceSequence' in attempt.payload, false);
});

test('设备、作业范围种类与引用走请求头，不进报文', () => {
  const attempt = prepared({
    spec: specOf('pick'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 1,
    values: { taskId: 'PKT-1', quantity: '2', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  });
  deepEqual(attempt.headers, { deviceId: device, scopeKind: 'WAREHOUSE', scopeReference: 'SYN-WH-01' });
  for (const key of ['deviceId', 'operator', 'tenantId', 'scopeKind', 'scopeReference']) {
    equal(key in attempt.payload, false, key);
  }
});

test('带固定载荷的作业（收货确认）叠上固定载荷，后面的工位直接用', () => {
  const attempt = prepared({
    spec: specOf('receipt-goods'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 3,
    values: {
      preAdviceId: 'SYN-IPA-01',
      lines: [{ observed: 'SYN-GOODS-01', unit: '件', stockState: 'GOOD', quantity: '6', locationCode: 'SYN-LOC-RCV' }],
    },
    now: at,
  });
  equal(attempt.payload.shape, 'BY_GOODS');
  equal(attempt.payload.occurredAt, '2026-11-25T08:30:15Z');
  equal('deviceSequence' in attempt.payload, false);
  deepEqual(attempt.payload.lines, [
    { observed: 'SYN-GOODS-01', unit: '件', stockState: 'GOOD', quantity: 6, locationCode: 'SYN-LOC-RCV' },
  ]);
});

test('要设备序号与发生时刻的作业，序号就是占下的那一个', () => {
  const attempt = prepared({
    spec: specOf('count-entry'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 12,
    values: { countId: 'CNT-1', locationCode: 'SYN-LOC-STORE', lines: [] },
    now: at,
  });
  equal(attempt.payload.deviceSequence, 12);
  equal(attempt.payload.occurredAt, '2026-11-25T08:30:15Z');
  equal(attempt.sequence, 12);
});

test('装不出来时说是哪一格不合，不送出去', () => {
  const zero = prepareWorkFact({
    spec: specOf('pick'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 1,
    values: { taskId: 'PKT-1', quantity: '0', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  });
  equal(zero.ok, false);
  if (!zero.ok) ok(zero.reason.includes('实拣数量'), zero.reason);

  const missing = prepareWorkFact({
    spec: specOf('pick'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 1,
    values: { quantity: '2', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  });
  equal(missing.ok, false);
});

// —— 事实号：铸一次，不重号 ——

test('铸一笔：占下本机序号并推进，事实号里带这个序号；旧页读到的就是推进后的值', () => {
  const store = memory();
  advanceDeviceSequence(store, device, 7);
  const result = mintWorkFact(store, {
    spec: specOf('pick'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    values: { taskId: 'PKT-1', quantity: '1', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  });
  ok(result.ok);
  if (!result.ok) return;
  equal(result.attempt.sequence, 8);
  equal(result.attempt.factId, 'PDA-SYN-PDA-WH01-01-20261125083015-8');
  equal(readDeviceSequence(store, device), 8);
});

// 库存流水按（原因，来源事实号）判重：同一秒里连铸两笔拣货若得到同一个事实号，第二笔会被当成第一笔的重放，货不挪。
test('同一台设备同一秒里连铸两笔，事实号不同（拣货本身不带设备序号也一样）', () => {
  const store = memory();
  const input = {
    spec: specOf('pick'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    values: { taskId: 'PKT-1', quantity: '1', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  };
  const first = mintWorkFact(store, input);
  const second = mintWorkFact(store, { ...input, values: { ...input.values, taskId: 'PKT-2' } });
  ok(first.ok && second.ok);
  if (!first.ok || !second.ok) return;
  notEqual(first.attempt.factId, second.attempt.factId);
  equal(second.attempt.sequence, first.attempt.sequence + 1);
});

test('本机存储读不了写不了时，靠这个页面里上一笔的序号不重号', () => {
  const input = {
    spec: specOf('pick'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    values: { taskId: 'PKT-1', quantity: '1', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  };
  const first = mintWorkFact(null, input, 0);
  ok(first.ok);
  if (!first.ok) return;
  const second = mintWorkFact(null, input, first.attempt.sequence);
  ok(second.ok);
  if (!second.ok) return;
  notEqual(first.attempt.factId, second.attempt.factId);
  ok(second.attempt.sequence > first.attempt.sequence);
});

test('不同设备的序号各数各的', () => {
  const store = memory();
  const input = {
    spec: specOf('pick'),
    warehouse: 'SYN-WH-01',
    values: { taskId: 'PKT-1', quantity: '1', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  };
  const a = mintWorkFact(store, { ...input, deviceId: 'DEV-A' });
  const b = mintWorkFact(store, { ...input, deviceId: 'DEV-B' });
  ok(a.ok && b.ok);
  if (!a.ok || !b.ok) return;
  equal(a.attempt.sequence, 1);
  equal(b.attempt.sequence, 1);
});

// —— 送出与重送 ——

test('送出：原样把路径、报文与请求头交给 postWorkFact；重送是同一个 attempt 再送一次，参数一个字不变', async () => {
  const attempt = prepared({
    spec: specOf('pick'),
    deviceId: device,
    warehouse: 'SYN-WH-01',
    sequence: 4,
    values: { taskId: 'PKT-1', quantity: '3', targetLocation: 'SYN-LOC-OUT-01' },
    now: at,
  });
  const calls: Array<{ path: string; payload: unknown; headers: unknown }> = [];
  let first = true;
  const post: WorkFactPoster = async (path, payload, headers) => {
    calls.push({ path, payload: JSON.parse(JSON.stringify(payload)), headers });
    if (first) {
      first = false;
      return { kind: 'transport', message: '网络断了' };
    }
    return { kind: 'outcome', status: 200, body: { outcome: 'EXISTING_RESULT' } };
  };
  const firstAnswer = await sendWorkFact(attempt, post);
  equal(firstAnswer.kind, 'transport');
  const secondAnswer = await sendWorkFact(attempt, post);
  equal(secondAnswer.kind, 'outcome');
  equal(calls.length, 2);
  deepEqual(calls[1], calls[0]);
  equal(calls[0].path, '/warehouse-fulfillment-picks');
  deepEqual(calls[0].headers, { deviceId: device, scopeKind: 'WAREHOUSE', scopeReference: 'SYN-WH-01' });
});

// —— 答复分类 ——

const outcomeAnswer = (outcome: string) => ({ kind: 'outcome', status: 200, body: { outcome } }) as const;

// 每个业务答复都要有一个归属：新增一个词（后端加了结果、work-fact.ts 的词表加了一格）而这里没有决定它算成还是没成，这条就红。
const doneOutcomes = [
  'CONFIRMED',
  'CORRECTED',
  'DIVERTED_TO_UNCLAIMED',
  'RECORDED',
  'REGISTERED',
  'SHORTAGE_REGISTERED',
  'POSTED',
  'PENDING',
  'PICKED',
  'SORTED',
  'CHECKED',
  'CHECK_MISMATCH',
  'PACKED',
  'SUBMITTED',
  'COUNTED',
  'UNCLAIMED',
  'EXISTING_RESULT',
];
const refusedOutcomes = [
  // 打包之后提交委托没成的四个词：打包已记下，工位上这一笔没走完，不算成
  'OWNERSHIP_UNRESOLVED',
  'ADMISSION_PAUSED',
  'OTHER_PRODUCTION_AUTHORITY',
  'INPUT_NOT_ACCEPTED',
  // 出库确认的放行条件没满足，没有记下
  'WITHHELD',
  // 合并拣货拣出来的货还没分拣完，这一次复核没有记
  'SORT_PENDING',
  'CONTENT_CONFLICT',
  'CONTENT_INVALID',
  'NOT_ACCEPTED',
  'NOT_CONFIGURED',
  'WAREHOUSE_NOT_FOUND',
  'WAREHOUSE_NOT_ACCEPTING',
  'PRE_ADVICE_NOT_FOUND',
  'PRE_ADVICE_MISMATCH',
  'CARTON_NOT_FOUND',
  'UNAVAILABLE',
  'STATUS_MISMATCH',
  'WAVE_NOT_FOUND',
  'COUNT_NOT_FOUND',
];

test('词表里的每个业务答复都归了类，没有漏的也没有多的', () => {
  deepEqual([...doneOutcomes, ...refusedOutcomes].sort(), Object.keys(workFactOutcomeLabels).sort());
});

// 下面这几个词各自对着 Go 端哪个符号、哪个登记口答，核过再写进词表（internal/warehousefulfillment/application）：
//   CHECK_MISMATCH         WaveMismatch，复核口：实点数与已拣合计不等，复核照样记下，出库单仍是已拣货
//   WITHHELD               WaveWithheld，出库确认口与自提交付口（Confirm）：不是离线交出、放行条件有缺口，没有记下、货没动
//   OWNERSHIP_UNRESOLVED / ADMISSION_PAUSED / OTHER_PRODUCTION_AUTHORITY / INPUT_NOT_ACCEPTED
//                          WaveUnresolved、WavePaused、WaveOtherAuthority、WaveInputRejected，打包口：只有打包之后提交委托那一步
//                          （CommissionHandler）才会答，打包已经记下，委托没提交成
// 词表里本来的 MISMATCH 后端从不答（复核不符答的是 CHECK_MISMATCH），换掉了。
test('复核数量不符答 CHECK_MISMATCH：这一次复核记下了，算成；老词表里后端从不答的 MISMATCH 不再有', () => {
  const verdict = judgeWorkFactAnswer(outcomeAnswer('CHECK_MISMATCH'));
  equal(verdict.verdict, 'done');
  equal(verdict.outcome, 'CHECK_MISMATCH');
  ok(verdict.headline.includes('已记下') && verdict.headline.includes('不能打包'));
  equal(outcomeRecorded('CHECK_MISMATCH'), true);
  equal(outcomeRecorded('MISMATCH'), false);
  equal('MISMATCH' in workFactOutcomeLabels, false);
});

test('放行条件没满足（WITHHELD）没有记下；打包之后委托没提交的四个词不算成，话里说打包已记下、委托没提交', () => {
  const withheld = judgeWorkFactAnswer(outcomeAnswer('WITHHELD'));
  equal(withheld.verdict, 'refused');
  equal(outcomeRecorded('WITHHELD'), false);
  ok(withheld.headline.includes('没有确认出库') && withheld.headline.includes('离线已交出'));
  for (const outcome of ['OWNERSHIP_UNRESOLVED', 'ADMISSION_PAUSED', 'OTHER_PRODUCTION_AUTHORITY', 'INPUT_NOT_ACCEPTED']) {
    const verdict = judgeWorkFactAnswer(outcomeAnswer(outcome));
    equal(verdict.verdict, 'refused', outcome);
    equal(outcomeRecorded(outcome), false, outcome);
    ok(verdict.headline.includes('打包已记下') && verdict.headline.includes('没提交本产品网络委托'), outcome);
  }
});

test('词表里的话都是中文，不带英文答复名', () => {
  for (const [outcome, text] of Object.entries(workFactOutcomeLabels)) {
    ok(text.length > 0, outcome);
    ok(!/[A-Z]{2,}_[A-Z_]+/.test(text), `${outcome}：${text}`);
  }
});

test('记下了的答复算成了：这一次记的、更早已记的（已有结果）、迟到记下的都算', () => {
  for (const outcome of doneOutcomes) {
    const verdict = judgeWorkFactAnswer(outcomeAnswer(outcome));
    equal(verdict.verdict, 'done', outcome);
    equal(verdict.outcome, outcome);
    ok(verdict.headline.length > 0, outcome);
    equal(outcomeRecorded(outcome), true, outcome);
  }
});

test('业务上不收的答复算没成别再送', () => {
  for (const outcome of refusedOutcomes) {
    const verdict = judgeWorkFactAnswer(outcomeAnswer(outcome));
    equal(verdict.verdict, 'refused', outcome);
    equal(verdict.outcome, outcome);
    equal(outcomeRecorded(outcome), false, outcome);
  }
});

test('业务答复照 workFactOutcomeLabels 说话', () => {
  equal(judgeWorkFactAnswer(outcomeAnswer('PICKED')).headline, workFactOutcomeLabels.PICKED);
  equal(judgeWorkFactAnswer(outcomeAnswer('CONTENT_INVALID')).headline, workFactOutcomeLabels.CONTENT_INVALID);
});

test('工位可以给个别答复换更贴切的说法，没列的退回默认；换说法不改变算成还是没成', () => {
  const labels = { EXISTING_RESULT: '这一格之前已经记过了', CONTENT_CONFLICT: '这一格已经拣过了' };
  const done = judgeWorkFactAnswer(outcomeAnswer('EXISTING_RESULT'), labels);
  equal(done.verdict, 'done');
  equal(done.headline, '这一格之前已经记过了');
  const refused = judgeWorkFactAnswer(outcomeAnswer('CONTENT_CONFLICT'), labels);
  equal(refused.verdict, 'refused');
  equal(refused.headline, '这一格已经拣过了');
  equal(judgeWorkFactAnswer(outcomeAnswer('PICKED'), labels).headline, workFactOutcomeLabels.PICKED);
});

test('不认识的业务答复不当成功，原词只进技术详情', () => {
  const verdict = judgeWorkFactAnswer(outcomeAnswer('SOMETHING_NEW'));
  equal(verdict.verdict, 'refused');
  ok(!verdict.headline.includes('SOMETHING_NEW'));
  equal(verdict.technical?.code, 'SOMETHING_NEW');
});

test('接入被拒按现有的说法：重送不会好，别再送', () => {
  const unconfigured = judgeWorkFactAnswer({ kind: 'unconfigured' });
  equal(unconfigured.verdict, 'refused');
  equal(unconfigured.access, 'unconfigured');
  ok(unconfigured.headline.includes('接入渠道未配置'));
  equal(unconfigured.technical?.status, 403);

  const login = judgeWorkFactAnswer({ kind: 'loginRequired' });
  equal(login.verdict, 'refused');
  equal(login.access, 'loginRequired');
  ok(login.headline.includes('需要登录'));
  equal(login.technical?.status, 401);

  const notGranted = judgeWorkFactAnswer({ kind: 'notGranted' });
  equal(notGranted.verdict, 'refused');
  equal(notGranted.access, 'notGranted');
  ok(notGranted.headline.includes('没有授予'));
});

test('接入拒绝的明面上不出状态码与错误码，那两样收进技术详情', () => {
  for (const kind of ['unconfigured', 'loginRequired', 'notGranted'] as const) {
    const verdict = judgeWorkFactAnswer({ kind });
    ok(!/ACCESS_CHANNEL|OPERATOR_/.test(verdict.headline), verdict.headline);
    ok(!/\b40[13]\b/.test(verdict.headline), verdict.headline);
    ok(verdict.technical?.code, kind);
  }
});

test('问题码按 workFactProblemNote 说，没成别再送', () => {
  const device = judgeWorkFactAnswer({ kind: 'callerProblem', status: 403, code: 'DEVICE_NOT_IN_USE' });
  equal(device.verdict, 'refused');
  ok(device.headline.includes('扫描设备'));
  deepEqual(device.technical, { status: 403, code: 'DEVICE_NOT_IN_USE' });

  const malformed = judgeWorkFactAnswer({ kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST', detail: 'x' });
  equal(malformed.verdict, 'refused');
  ok(malformed.headline.includes('报文'));
});

test('没送到与后端没能处理可以再送，并说清用的是同一个事实号', () => {
  const noAnswer = judgeWorkFactAnswer({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' });
  equal(noAnswer.verdict, 'retry');
  ok(noAnswer.headline.includes('再送一次'));
  ok(noAnswer.headline.includes('同一个作业事实号'));
  deepEqual(noAnswer.technical, { status: 500, code: 'NO_ANSWER_FORMED' });

  const transport = judgeWorkFactAnswer({ kind: 'transport', message: 'Failed to fetch' });
  equal(transport.verdict, 'retry');
  ok(transport.headline.includes('再送一次'));
  ok(!transport.headline.includes('Failed to fetch'));
  equal(transport.technical?.message, 'Failed to fetch');
});

test('读口没问成的原因说成人话', () => {
  equal(describeReadFailure({ kind: 'unconfigured' }), '接入渠道未配置');
  equal(describeReadFailure({ kind: 'loginRequired' }), '需要登录');
  equal(describeReadFailure({ kind: 'notGranted' }), '没有授予');
  ok(describeReadFailure({ kind: 'transport', message: '断了' }).includes('网络没通'));
  ok(describeReadFailure({ kind: 'noAnswer', status: 500, code: 'X' }).includes('没答出来'));
  ok(describeReadFailure({ kind: 'callerProblem', status: 403, code: 'DEVICE_NOT_IN_USE' }).includes('扫描设备'));
});
