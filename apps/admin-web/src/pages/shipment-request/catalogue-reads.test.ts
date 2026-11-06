import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  findAcceptanceReviewCase,
  findPostIntakeManualItemsOfParcel,
  findShipmentRequestView,
  findShipmentRequestViewsByDocumentNumber,
  findShipmentRequestViewsByIds,
  listAcceptanceReviewQueue,
  listAuthorizedDispositionQueue,
  listControlReleaseRefusals,
  listLabelTransactions,
  listPostIntakeDispositionRequests,
  listPostIntakeManualItems,
  listShipmentRequestViews,
} from './api';
import type { CatalogueConditions } from '../catalogue-query';

// 本文件钉小包托运各列表读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144
// 决定一、四、五）：不带参数即裸路径，带条件时经 catalogueRequestPath 拼成同一个查询串；有界查找的请求一字不变。

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

// Covers: 运单列表——裸路径照旧（工作台与鉴权测试的无参调用）；检索词、状态维、排序与游标一起下推，键按字典序、游标殿后。
test('运单列表不带参数即裸路径，带检索、状态、排序与游标时一并下推', async () => {
  const urls = captureRequests();
  await listShipmentRequestViews();
  await listShipmentRequestViews({ q: ' P2026 ', filters: { state: ['ACCEPTED'] }, sort: 'submittedAt' }, 'c1');
  deepEqual(urls, [
    '/shipment-request-views',
    '/shipment-request-views?sort=submittedAt&state=ACCEPTED&q=P2026&after=c1',
  ]);
});

// Covers: 两个队列、释放未受理册、等回应的停止请求、待登记项与面单交易各自裸路径照旧（工作台待办格的无参调用），带检索词与
// 游标时一并下推。
test('其余列表读口不带参数即裸路径，带检索词与游标时一并下推', async () => {
  const reads: Array<[string, (conditions?: CatalogueConditions, after?: string | null) => Promise<unknown>]> = [
    ['/acceptance-review-queue', listAcceptanceReviewQueue],
    ['/authorized-disposition-queue', listAuthorizedDispositionQueue],
    ['/control-release-refusals', listControlReleaseRefusals],
    ['/post-intake-disposition-requests', listPostIntakeDispositionRequests],
    ['/post-intake-disposition-manual-items', listPostIntakeManualItems],
    ['/label-transactions', listLabelTransactions],
  ];
  for (const [path, read] of reads) {
    const urls = captureRequests();
    await read();
    await read({ q: '合成' }, 'c1');
    deepEqual(urls, [path, `${path}?q=%E5%90%88%E6%88%90&after=c1`], path);
  }
});

// Covers: 筛选维按声明的参数名下推，同一维多个值去重、升序、逐个带上（维内为或）。
test('筛选维按参数名下推，同一维多个值逐个带上', async () => {
  const urls = captureRequests();
  await listLabelTransactions({ filters: { transactionResult: ['FAILED', 'RESULT_UNCERTAIN', 'FAILED'] } });
  await listPostIntakeManualItems({ filters: { owner: ['NODE_OPERATIONS'], action: ['HOLD_AT_CONTROL'] } });
  deepEqual(urls, [
    '/label-transactions?transactionResult=FAILED&transactionResult=RESULT_UNCERTAIN',
    '/post-intake-disposition-manual-items?action=HOLD_AT_CONTROL&owner=NODE_OPERATIONS',
  ]);
});

// Covers: 有界查找（按运单号、按一组标识、按标识取单份、复核单份、按包裹读回）不翻页，请求与迁移前一字不变。
test('有界查找的请求一字不变', async () => {
  const urls = captureRequests();
  await findShipmentRequestViewsByDocumentNumber('P20261102000001');
  await findShipmentRequestViewsByIds(['SHR-1', 'SHR-2']);
  await findShipmentRequestView('SHR-1');
  await findAcceptanceReviewCase('SHR-1');
  await findPostIntakeManualItemsOfParcel('DPCL-PARCEL1');
  deepEqual(urls, [
    '/shipment-request-views?documentNumber=P20261102000001',
    '/shipment-request-views?shipmentRequestIds=SHR-1,SHR-2',
    '/shipment-request-views?shipmentRequestId=SHR-1',
    '/acceptance-review-queue?shipmentRequestId=SHR-1',
    '/post-intake-disposition-manual-items?parcel=DPCL-PARCEL1',
  ]);
});

// Covers: 答复的 page 原样交出，翻页条据它算页数与下一页。
test('答复的 page 原样交出', async () => {
  captureRequests();
  const result = await listControlReleaseRefusals();
  if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
  deepEqual(result.body.page, page);
});

// Covers: 读口对集外的键、坏游标答 400 并带一句理由散文——callerProblem 原样带上 detail；服务端没给字符串时不多出这一格。
test('400 带回的理由散文原样进 detail，没给就没有这一格', async () => {
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '游标与本次的排序或筛选不符，请从第一页重取' } }),
      { status: 400 },
    )) as typeof fetch;
  deepEqual(await listShipmentRequestViews({}, 'stale'), {
    kind: 'callerProblem',
    status: 400,
    code: 'MALFORMED_REQUEST',
    detail: '游标与本次的排序或筛选不符，请从第一页重取',
  });

  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST' } }), { status: 400 })) as typeof fetch;
  const bare = await listAcceptanceReviewQueue();
  deepEqual(bare, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  equal('detail' in bare, false);
});
