import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  findAdjustment,
  findFreeze,
  findInstruction,
  findJobDiscrepancy,
  getCountDetail,
  getInternalJob,
  getStockImportDetail,
  getUnclaimedArrivalDetail,
} from './api';

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

function answerWith(status: number, body: unknown): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
  return urls;
}

// 单据页按号取一张的两个读口。建账导入的读口答导入本身而不是带 outcome 的外壳，查无此号答 404；页面不认问题码，
// 所以 api 层把它收成与别的按号读口同一个形状。

test('建账导入按导入号取一张，号里的特殊字符照编码，答的导入本身包成 found', async () => {
  const record = { importId: 'IMP/1 一', status: 'PENDING', goodsLines: [], cartonLines: [] };
  const urls = answerWith(200, record);
  const answer = await getStockImportDetail('IMP/1 一');
  deepEqual(urls, ['/warehouse-fulfillment-stock-import?importId=IMP%2F1+%E4%B8%80']);
  deepEqual(answer, { kind: 'outcome', status: 200, body: { found: true, stockImport: record } });
});

test('建账导入查无此号（404 STOCK_IMPORT_NOT_FOUND）收成没有这张单，不是读取失败', async () => {
  answerWith(404, { error: { code: 'STOCK_IMPORT_NOT_FOUND' } });
  deepEqual(await getStockImportDetail('IMP-9'), { kind: 'outcome', status: 404, body: { found: false } });
});

test('建账导入别的非业务答复原样交回：别的 404 码、没授予、服务端没形成答案都不冒充没有这张单', async () => {
  answerWith(404, { error: { code: 'SOMETHING_ELSE' } });
  const other = await getStockImportDetail('IMP-9');
  equal(other.kind, 'callerProblem');

  answerWith(403, { error: { code: 'OPERATOR_NOT_GRANTED' } });
  equal((await getStockImportDetail('IMP-9')).kind, 'notGranted');

  answerWith(500, { error: { code: 'NO_ANSWER_FORMED' } });
  equal((await getStockImportDetail('IMP-9')).kind, 'noAnswer');
});

test('待认领到货按到货号取一张，答复照原样交回（found 由读口自己说）', async () => {
  const body = { outcome: 'UNCLAIMED_ARRIVAL_READ', arrivalId: 'UCA-1', found: false };
  const urls = answerWith(200, body);
  const answer = await getUnclaimedArrivalDetail('UCA-1');
  deepEqual(urls, ['/warehouse-fulfillment-unclaimed-arrival?arrivalId=UCA-1']);
  deepEqual(answer, { kind: 'outcome', status: 200, body });
});

test('盘点按盘点号取一张，查无此号是业务答复（count 缺席），原样交回', async () => {
  const body = { outcome: 'COUNT_NOT_FOUND' };
  const urls = answerWith(200, body);
  const answer = await getCountDetail('CNT/1');
  deepEqual(urls, ['/warehouse-fulfillment-count?countId=CNT%2F1']);
  deepEqual(answer, { kind: 'outcome', status: 200, body });
});

test('库内作业按作业事实号取一次，答复照原样交回', async () => {
  const body = { outcome: 'READ', factId: 'FACT-1', found: true, job: { factId: 'FACT-1', kind: 'INTERNAL_MOVE', lines: [] } };
  const urls = answerWith(200, body);
  deepEqual(await getInternalJob('FACT-1'), { kind: 'outcome', status: 200, body });
  deepEqual(urls, ['/warehouse-fulfillment-internal-job?factId=FACT-1']);
});

// 库存调整与作业差异没有按号取一张的读口：按号检索列表，再在答回来的那一页里认出号完全相同的那一张。

const adjustmentPage = (ids: string[], next: string | null = null) => ({
  outcome: 'ADJUSTMENTS_LISTED',
  adjustments: ids.map((adjustmentId) => ({ adjustmentId, status: 'PENDING', lines: [] })),
  page: { size: 200, next, total: ids.length },
});

test('库存调整按号检索，在答回来的一页里认出号完全相同的那张：包含匹配带出的别的单不冒充', async () => {
  const urls = answerWith(200, adjustmentPage(['ADJ-10', 'ADJ-1', 'ADJ-11']));
  const answer = await findAdjustment('ADJ-1');
  deepEqual(urls, ['/warehouse-fulfillment-adjustments?q=ADJ-1']);
  equal(answer.kind, 'outcome');
  if (answer.kind !== 'outcome') return;
  equal(answer.body.record?.adjustmentId, 'ADJ-1');
  equal(answer.body.searched, 3);
  equal(answer.body.truncated, false);
});

test('库存调整检索里没有这个号：record 缺席；后面还有页时说没翻到，不说成没有这张单', async () => {
  answerWith(200, adjustmentPage(['ADJ-10', 'ADJ-11']));
  const absent = await findAdjustment('ADJ-1');
  equal(absent.kind === 'outcome' && absent.body.record, undefined);
  equal(absent.kind === 'outcome' && absent.body.truncated, false);

  answerWith(200, adjustmentPage(['ADJ-10', 'ADJ-11'], 'CURSOR-2'));
  const unfinished = await findAdjustment('ADJ-1');
  equal(unfinished.kind === 'outcome' && unfinished.body.record, undefined);
  equal(unfinished.kind === 'outcome' && unfinished.body.truncated, true);
  equal(unfinished.kind === 'outcome' && unfinished.body.searched, 2);
});

test('库存调整检索的号里有特殊字符照编码；读口的非业务答复原样交回', async () => {
  const urls = answerWith(200, adjustmentPage([]));
  await findAdjustment('ADJ/1 一');
  deepEqual(urls, ['/warehouse-fulfillment-adjustments?q=ADJ%2F1+%E4%B8%80']);

  answerWith(403, { error: { code: 'OPERATOR_NOT_GRANTED' } });
  equal((await findAdjustment('ADJ-1')).kind, 'notGranted');
  answerWith(500, { error: { code: 'NO_ANSWER_FORMED' } });
  equal((await findAdjustment('ADJ-1')).kind, 'noAnswer');
});

test('作业差异按号检索，同样只认号完全相同的那一条', async () => {
  const urls = answerWith(200, {
    outcome: 'JOB_DISCREPANCIES_LISTED',
    discrepancies: [{ discrepancyId: 'SDP-B', factId: 'SDP-A' }, { discrepancyId: 'SDP-A', factId: 'F-1' }],
    page: { size: 200, next: null, total: 2 },
  });
  const answer = await findJobDiscrepancy('SDP-A');
  deepEqual(urls, ['/warehouse-fulfillment-job-discrepancies?q=SDP-A']);
  equal(answer.kind === 'outcome' && answer.body.record?.discrepancyId, 'SDP-A');
  equal(answer.kind === 'outcome' && answer.body.record?.factId, 'F-1');
});

test('作业差异检索不到：record 缺席，传输失败原样交回', async () => {
  answerWith(200, { outcome: 'JOB_DISCREPANCIES_LISTED', discrepancies: [], page: { size: 200, next: null, total: 0 } });
  const absent = await findJobDiscrepancy('SDP-X');
  equal(absent.kind === 'outcome' && absent.body.record, undefined);

  globalThis.fetch = (async () => {
    throw new Error('网络断了');
  }) as typeof fetch;
  deepEqual(await findJobDiscrepancy('SDP-X'), { kind: 'transport', message: '网络断了' });
});

// 加工与换标指令、库存冻结同样没有按号取一张的读口，退回按号检索列表。

const instructionPage = (ids: string[], next: string | null = null) => ({
  outcome: 'INSTRUCTIONS_LISTED',
  instructions: ids.map((instructionId) => ({ instructionId, status: 'PENDING', components: [], executions: [] })),
  page: { size: 200, next, total: ids.length },
});

test('指令按号检索，只认号完全相同的那一条：包含匹配带出的别的指令不冒充', async () => {
  const urls = answerWith(200, instructionPage(['INS-10', 'INS-1', 'INS-11']));
  const answer = await findInstruction('INS-1');
  deepEqual(urls, ['/warehouse-fulfillment-instructions?q=INS-1']);
  equal(answer.kind === 'outcome' && answer.body.record?.instructionId, 'INS-1');
  equal(answer.kind === 'outcome' && answer.body.searched, 3);
  equal(answer.kind === 'outcome' && answer.body.truncated, false);
});

test('指令检索里没有这个号：record 缺席；后面还有页时说没翻到，不说成没有这条指令', async () => {
  answerWith(200, instructionPage(['INS-10', 'INS-11']));
  const absent = await findInstruction('INS-1');
  equal(absent.kind === 'outcome' && absent.body.record, undefined);
  equal(absent.kind === 'outcome' && absent.body.truncated, false);

  answerWith(200, instructionPage(['INS-10', 'INS-11'], 'CURSOR-2'));
  const unfinished = await findInstruction('INS-1');
  equal(unfinished.kind === 'outcome' && unfinished.body.record, undefined);
  equal(unfinished.kind === 'outcome' && unfinished.body.truncated, true);
  equal(unfinished.kind === 'outcome' && unfinished.body.searched, 2);
});

test('指令检索的号里有特殊字符照编码；读口的非业务答复原样交回', async () => {
  const urls = answerWith(200, instructionPage([]));
  await findInstruction('INS/1 一');
  deepEqual(urls, ['/warehouse-fulfillment-instructions?q=INS%2F1+%E4%B8%80']);

  answerWith(403, { error: { code: 'OPERATOR_NOT_GRANTED' } });
  equal((await findInstruction('INS-1')).kind, 'notGranted');
  answerWith(500, { error: { code: 'NO_ANSWER_FORMED' } });
  equal((await findInstruction('INS-1')).kind, 'noAnswer');
});

const freezePage = (ids: string[], next: string | null = null) => ({
  outcome: 'FREEZES_LISTED',
  freezes: ids.map((freezeId) => ({ freezeId, status: 'FROZEN', target: 'GOODS', quantity: 1 })),
  page: { size: 200, next, total: ids.length },
});

test('冻结按号检索，只认号完全相同的那一份；认不出而后面还有页时说没翻到', async () => {
  const urls = answerWith(200, freezePage(['FRZ-2', 'FRZ-20', 'FRZ-200']));
  const answer = await findFreeze('FRZ-20');
  deepEqual(urls, ['/warehouse-fulfillment-freezes?q=FRZ-20']);
  equal(answer.kind === 'outcome' && answer.body.record?.freezeId, 'FRZ-20');
  equal(answer.kind === 'outcome' && answer.body.searched, 3);

  answerWith(200, freezePage(['FRZ-200'], 'CURSOR-2'));
  const unfinished = await findFreeze('FRZ-20');
  equal(unfinished.kind === 'outcome' && unfinished.body.record, undefined);
  equal(unfinished.kind === 'outcome' && unfinished.body.truncated, true);

  answerWith(200, freezePage([]));
  const absent = await findFreeze('FRZ-20');
  equal(absent.kind === 'outcome' && absent.body.record, undefined);
  equal(absent.kind === 'outcome' && absent.body.truncated, false);
});

test('冻结检索的传输失败与没授予原样交回', async () => {
  globalThis.fetch = (async () => {
    throw new Error('网络断了');
  }) as typeof fetch;
  deepEqual(await findFreeze('FRZ-1'), { kind: 'transport', message: '网络断了' });

  answerWith(403, { error: { code: 'OPERATOR_NOT_GRANTED' } });
  equal((await findFreeze('FRZ-1')).kind, 'notGranted');
});
