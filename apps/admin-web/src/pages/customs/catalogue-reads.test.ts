import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  CASE_LOOKUP_BATCH,
  findCustomsCasesById,
  listAdmissionJudgements,
  listCaseRegisters,
  listComplianceRules,
  listCredentials,
  listCustomsCases,
  listDutyCollaborations,
  listDutyVerifications,
  listExternalResults,
  listGateConditions,
  listPortsPaths,
  listSubmissionVersions,
  listUnadmittedObjects,
  registryConditions,
} from './api';
import { everyPage } from './every-page';

// 本文件钉关务各列表读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144
// 决定一、四、五）：按册分派的三个读口把 registry 当选择器与条件一起拼进查询串，不带条件即迁移前的路径；伴随读走到末页、
// 按案件标识读回清关单分批问。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 2, next: 'c2', total: 3 };

function captureRequests(): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify({ outcome: 'ANY', page }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return urls;
}

/** 按请求里的游标答下一页：pages 逐页给各行的来源标识，最后一页的 next 为 null。 */
function pagedRequests(key: string, pages: readonly (readonly string[])[]): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    urls.push(url);
    const after = new URL(url, 'http://local').searchParams.get('after');
    const index = after === null ? 0 : Number(after.slice(1));
    const next = index + 1 < pages.length ? `p${index + 1}` : null;
    const rows = pages[index].map((source) => ({ source }));
    return new Response(JSON.stringify({ outcome: 'ANY', [key]: rows, page: { size: 2, next, total: null } }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return urls;
}

// Covers: 合规规则、案件配置、口岸与申报路径三个按册分派的读口只带册名时，路径与迁移前一样（口岸名称查名就这么调）。
test('按册分派的三个读口只带册名时路径同迁移前', async () => {
  const urls = captureRequests();
  await listComplianceRules(registryConditions('case-requirement'));
  await listComplianceRules(registryConditions('interpretation'));
  await listCaseRegisters(registryConditions('readiness'));
  await listCaseRegisters(registryConditions('submission-authority'));
  await listCaseRegisters(registryConditions('closure-obligation'));
  await listPortsPaths(registryConditions('candidate-port'));
  await listPortsPaths(registryConditions('declaration-path'));
  deepEqual(urls, [
    '/customs-compliance-rules?registry=case-requirement',
    '/customs-compliance-rules?registry=interpretation',
    '/customs-case-registers?registry=readiness',
    '/customs-case-registers?registry=submission-authority',
    '/customs-case-registers?registry=closure-obligation',
    '/customs-ports-paths?registry=candidate-port',
    '/customs-ports-paths?registry=declaration-path',
  ]);
});

// Covers: 册名与排序、筛选维、检索词、游标同进一个查询串——选择器在前、筛选维按名、检索词去首尾空白、游标殿后。
test('册名与排序、筛选维、检索词、游标一并下推', async () => {
  const urls = captureRequests();
  await listComplianceRules(registryConditions('interpretation', { q: ' SYN-JUR ', filters: { layer: ['REGULATORY_RECEIPT'] } }), 'c1');
  await listPortsPaths(registryConditions('declaration-path', { sort: '-appliesFrom', filters: { direction: ['IMPORT', 'EXPORT'] } }));
  await listCaseRegisters(registryConditions('closure-obligation', { q: 'SYN-CASE' }), 'c2');
  deepEqual(urls, [
    '/customs-compliance-rules?registry=interpretation&layer=REGULATORY_RECEIPT&q=SYN-JUR&after=c1',
    '/customs-ports-paths?registry=declaration-path&sort=-appliesFrom&direction=EXPORT&direction=IMPORT',
    '/customs-case-registers?registry=closure-obligation&q=SYN-CASE&after=c2',
  ]);
});

// Covers: 单册读口不收册名：不带参数即裸路径（首页待办数卡关件数就这么调），带条件与游标时一并下推。
test('单册读口不带参数即裸路径，带条件与游标时一并下推', async () => {
  const urls = captureRequests();
  await listGateConditions();
  await listCredentials();
  await listDutyCollaborations();
  await listDutyVerifications();
  await listCustomsCases();
  await listSubmissionVersions();
  await listExternalResults();
  await listAdmissionJudgements();
  await listUnadmittedObjects();
  await listGateConditions({ q: 'SYN-SCOPE', filters: { action: ['FINAL_DELIVERY'] } }, 'c1');
  await listDutyVerifications({ filters: { delta: ['SHORT'], coverage: ['PARTIAL'] } });
  await listCustomsCases({ q: 'SYN-PCL-01', sort: '-establishedAt' }, 'c2');
  await listSubmissionVersions({ filters: { isCurrent: ['true'] } }, 'c3');
  await listUnadmittedObjects({ q: 'SYN-NODE' }, 'c4');
  deepEqual(urls, [
    '/customs-gate-conditions',
    '/customs-credentials',
    '/customs-duty-collaborations',
    '/customs-duty-verifications',
    '/customs-case-records',
    '/customs-submission-records',
    '/customs-external-result-records',
    '/customs-action-admission-judgements',
    '/customs-unadmitted-objects',
    '/customs-gate-conditions?action=FINAL_DELIVERY&q=SYN-SCOPE&after=c1',
    '/customs-duty-verifications?coverage=PARTIAL&delta=SHORT',
    '/customs-case-records?sort=-establishedAt&q=SYN-PCL-01&after=c2',
    '/customs-submission-records?isCurrent=true&after=c3',
    '/customs-unadmitted-objects?q=SYN-NODE&after=c4',
  ]);
});

// Covers: 放行记录按包裹号查走筛选维 object，请求与迁移前那一问同形；空串不发这一维（空的筛选值读口答 400）。
test('放行记录按包裹号查走筛选维 object', async () => {
  const urls = captureRequests();
  await listAdmissionJudgements({ filters: { object: ['SYN-PCL-01'] } });
  await listAdmissionJudgements({ q: 'SYN-NODE', filters: { object: [] } }, 'c1');
  deepEqual(urls, ['/customs-action-admission-judgements?object=SYN-PCL-01', '/customs-action-admission-judgements?q=SYN-NODE&after=c1']);
});

// Covers: 答复的 page 原样交出，翻页条据它算页数与下一页。
test('答复的 page 原样交出', async () => {
  captureRequests();
  const result = await listPortsPaths(registryConditions('candidate-port'));
  if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
  deepEqual(result.body.page, page);
});

// Covers: 伴随读从第一页起带着上一页的 next 往下读到 next 为 null，各页的行按次序接成一份。
test('伴随读走到末页，各页的行接成一份', async () => {
  const urls = pagedRequests('results', [['r1', 'r2'], ['r3', 'r4'], ['r5']]);
  const answer = await everyPage((after) => listExternalResults({}, after), (body) => body.results);
  if (answer.kind !== 'outcome') throw new Error(`读口答 ${answer.kind}`);
  deepEqual(
    answer.body.map((result) => result.source),
    ['r1', 'r2', 'r3', 'r4', 'r5'],
  );
  deepEqual(urls, ['/customs-external-result-records', '/customs-external-result-records?after=p1', '/customs-external-result-records?after=p2']);
});

// Covers: 伴随读中途有一页没答出，交回那一页的答复，不拿前几页冒充全册。
test('伴随读中途一页没答出就交回那一页的答复', async () => {
  let calls = 0;
  globalThis.fetch = (async () => {
    calls += 1;
    return calls === 1
      ? new Response(JSON.stringify({ outcome: 'ANY', results: ['r1'], page: { size: 1, next: 'p1', total: 2 } }), { status: 200 })
      : new Response(JSON.stringify({ error: { code: 'NO_ANSWER_FORMED' } }), { status: 503 });
  }) as typeof fetch;
  const answer = await everyPage((after) => listExternalResults({}, after), (body) => body.results);
  deepEqual(answer, { kind: 'noAnswer', status: 503, code: 'NO_ANSWER_FORMED' });
});

// Covers: 按案件标识读回清关单——标识按筛选维 case 逐值下推，每批至多 CASE_LOOKUP_BATCH 个、各读到末页；空清单不发请求。
test('按案件标识读回清关单分批问，每批读到末页', async () => {
  const ids = Array.from({ length: CASE_LOOKUP_BATCH + 1 }, (_, index) => `SYN-CASE-${String(index).padStart(3, '0')}`);
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = new URL(String(input), 'http://local');
    urls.push(String(input));
    const asked = url.searchParams.getAll('case');
    // 第一批答两页，第二批一页答完。
    const firstBatch = asked.length === CASE_LOOKUP_BATCH;
    const after = url.searchParams.get('after');
    const cases = firstBatch ? (after === null ? asked.slice(0, 1) : asked.slice(1)) : asked;
    const next = firstBatch && after === null ? 'p1' : null;
    return new Response(
      JSON.stringify({ outcome: 'CUSTOMS_CASES_LISTED', cases: cases.map((id) => ({ case: id })), page: { size: 200, next, total: null } }),
      { status: 200 },
    );
  }) as typeof fetch;

  const answer = await findCustomsCasesById(ids);
  if (answer.kind !== 'outcome') throw new Error(`读口答 ${answer.kind}`);
  deepEqual(
    answer.body.map((record) => record.case),
    ids,
  );
  equal(urls.length, 3);
  deepEqual(new URL(urls[0], 'http://local').searchParams.getAll('case'), ids.slice(0, CASE_LOOKUP_BATCH));
  equal(new URL(urls[1], 'http://local').searchParams.get('after'), 'p1');
  deepEqual(new URL(urls[2], 'http://local').searchParams.getAll('case'), ids.slice(CASE_LOOKUP_BATCH));

  const none = captureRequests();
  deepEqual(await findCustomsCasesById([]), { kind: 'outcome', status: 200, body: [] });
  deepEqual(none, []);
});

// Covers: 读口对集外的键、坏游标答 400 并带一句理由散文——callerProblem 原样带上 detail；服务端没给字符串时不多出这一格。
test('400 带回的理由散文原样进 detail，没给就没有这一格', async () => {
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '游标与本次的排序或筛选不符，请从第一页重取' } }),
      { status: 400 },
    )) as typeof fetch;
  deepEqual(await listComplianceRules(registryConditions('interpretation'), 'stale'), {
    kind: 'callerProblem',
    status: 400,
    code: 'MALFORMED_REQUEST',
    detail: '游标与本次的排序或筛选不符，请从第一页重取',
  });

  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST' } }), { status: 400 })) as typeof fetch;
  const bare = await listGateConditions();
  deepEqual(bare, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  equal('detail' in bare, false);
});
