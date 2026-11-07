import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { findCurrentProjection, findProjectionVersion, listTrackingProjections } from './api';
import {
  claimsRecoveryConditions,
  exceptionTriageConditions,
  listClaimsRecoveryRecords,
  listExceptionCaseRecords,
  listExceptionTriageRecords,
} from './case-api';
import { listVisibilityCatalogues, visibilityCatalogueConditions } from './catalogue-api';

// 本文件钉物流动态与异常区各列表读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约
// ADR-0144 决定一、四、五）：不带条件即裸路径（工作台与鉴权测试的无参调用照旧）；registry、kind 是册子选择器，排在最前；
// 排序、筛选维、检索词、游标经 catalogueRequestPath 拼成同一个查询串。按包裹、按版本取单件的两支不翻页，请求一字不变。

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

// Covers: 物流动态——列表裸路径照旧；检索词与排序、游标一并下推；按包裹、按版本两支不带翻页参数。
test('物流动态列表不带参数即裸路径，带检索、排序与游标时一并下推；单件两支照旧', async () => {
  const urls = captureRequests();
  await listTrackingProjections();
  await listTrackingProjections({ q: ' DPCL ', sort: '-parcel' }, 'c1');
  await findCurrentProjection('DPCL-1');
  await findProjectionVersion('PRJ-1');
  deepEqual(urls, [
    '/tracking-projections',
    '/tracking-projections?sort=-parcel&q=DPCL&after=c1',
    '/tracking-projections?parcel=DPCL-1',
    '/tracking-projections?version=PRJ-1',
  ]);
});

// Covers: 异常案件——phase 一维逐值去重升序下推（维内为或），排序在筛选之前、游标殿后。
test('异常案件的 phase、排序、检索词与游标一并下推', async () => {
  const urls = captureRequests();
  await listExceptionCaseRecords();
  await listExceptionCaseRecords(
    { q: 'SYN', sort: 'rootParcel', filters: { phase: ['CLOSED', 'AWAITING_RESPONSE', 'CLOSED'] } },
    'c1',
  );
  deepEqual(urls, [
    '/exception-case-records',
    '/exception-case-records?sort=rootParcel&phase=AWAITING_RESPONSE&phase=CLOSED&q=SYN&after=c1',
  ]);
});

// Covers: 分诊两本册——registry 是选择器，只带它即该册第一页；分诊结论 outcome、处置答复 judgment 各是本册的筛选维。
test('分诊两本册以 registry 选册，筛选维与排序、检索词、游标跟在后面', async () => {
  const urls = captureRequests();
  await listExceptionTriageRecords(exceptionTriageConditions('signal-episode'));
  await listExceptionTriageRecords(
    exceptionTriageConditions('signal-episode', { q: 'DPCL', sort: 'lastHitAt', filters: { outcome: ['MANUAL_REVIEW'] } }),
    'c1',
  );
  await listExceptionTriageRecords(exceptionTriageConditions('disposition-request', { filters: { judgment: ['REFUSED'] } }));
  deepEqual(urls, [
    '/exception-triage-records?registry=signal-episode',
    '/exception-triage-records?registry=signal-episode&sort=lastHitAt&outcome=MANUAL_REVIEW&q=DPCL&after=c1',
    '/exception-triage-records?registry=disposition-request&judgment=REFUSED',
  ]);
});

// Covers: 索赔追偿三本册——registry 选册；索赔项的 screen、conclusion 两维按参数名排、维间为与。
test('索赔追偿三本册以 registry 选册，检索词与游标一并下推', async () => {
  const urls = captureRequests();
  await listClaimsRecoveryRecords(claimsRecoveryConditions('customer-notification'));
  await listClaimsRecoveryRecords(claimsRecoveryConditions('recovery-matter', { q: '合成' }), 'c1');
  await listClaimsRecoveryRecords(
    claimsRecoveryConditions('claim-item', { filters: { screen: ['ELIGIBLE'], conclusion: ['NOT_ESTABLISHED'] } }),
  );
  deepEqual(urls, [
    '/claims-recovery-records?registry=customer-notification',
    '/claims-recovery-records?registry=recovery-matter&q=%E5%90%88%E6%88%90&after=c1',
    '/claims-recovery-records?registry=claim-item&conclusion=NOT_ESTABLISHED&screen=ELIGIBLE',
  ]);
});

// Covers: 规则与策略目录——kind 是选择器（换册即换条件签名），版本册按生效起点排、按键的册按键排。
test('可见性目录以 kind 选册，排序、检索词与游标一并下推', async () => {
  const urls = captureRequests();
  await listVisibilityCatalogues(visibilityCatalogueConditions('MILESTONE_MAPPING'));
  await listVisibilityCatalogues(visibilityCatalogueConditions('TRIAGE_RULE', { q: 'SYN-VE', sort: 'effectiveFrom' }), 'c1');
  await listVisibilityCatalogues(visibilityCatalogueConditions('CLAIM_AUTHORIZATION', { sort: '-customer' }));
  deepEqual(urls, [
    '/visibility-catalogues?kind=MILESTONE_MAPPING',
    '/visibility-catalogues?kind=TRIAGE_RULE&sort=effectiveFrom&q=SYN-VE&after=c1',
    '/visibility-catalogues?kind=CLAIM_AUTHORIZATION&sort=-customer',
  ]);
});

// Covers: 答复的 page 原样交出，翻页条据它算页数与下一页——物流动态走本目录自己的出口，其余走共享的 exchangeMasterData。
test('答复的 page 原样交出', async () => {
  captureRequests();
  for (const result of [
    await listTrackingProjections(),
    await listExceptionCaseRecords(),
    await listExceptionTriageRecords(exceptionTriageConditions('disposition-request')),
    await listClaimsRecoveryRecords(claimsRecoveryConditions('claim-item')),
    await listVisibilityCatalogues(visibilityCatalogueConditions('NOTIFICATION_POLICY')),
  ]) {
    if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
    deepEqual(result.body.page, page);
  }
});

// Covers: 读口对集外的键、坏游标答 400 并带一句理由散文——两种出口的 callerProblem 都原样带上 detail；没给字符串时不多出这一格。
test('400 带回的理由散文原样进 detail，没给就没有这一格', async () => {
  const detail = '游标与本次的排序或筛选不符，请从第一页重取';
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail } }), { status: 400 })) as typeof fetch;
  deepEqual(await listTrackingProjections({}, 'stale'), { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST', detail });
  deepEqual(await listExceptionCaseRecords({}, 'stale'), { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST', detail });

  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST' } }), { status: 400 })) as typeof fetch;
  const bare = await listTrackingProjections();
  deepEqual(bare, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  equal('detail' in bare, false);
});
