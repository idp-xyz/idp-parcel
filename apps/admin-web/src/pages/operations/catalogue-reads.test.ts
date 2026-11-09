import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { bounceFilterDimensions } from './booking-registers';
import { deliveryScopeConditions, handoverNoteFilterKeys, preAdviceFilterKeys } from './delivery-pre-advice';
import { listDeliveryHandoverNotes, listDeliveryPreAdvices } from './delivery-scope-api';
import { externalTrackingFactConditions, listExternalTrackingFacts } from './effective-time-judgment-api';
import { effectiveBasisStatus, nodePushedStatus, reviewListConditions, transportPushedStatus } from './list-conditions';
import {
  listIdentityReviewUnits,
  listNodeOperationsRecords,
  listTransportFulfillmentRecords,
  nodeOperationsConditions,
  transportFulfillmentConditions,
} from './records-api';

// 本文件钉运输班次、网点作业与外部轨迹几条列表读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；
// 契约 ADR-0144 决定一、四、五）：registry、source / view、送站两区的节点与日子是选择器，按参数名排在最前；排序、筛选维、检索词、
// 游标经 catalogueRequestPath 拼成同一个查询串。条件经页面用的那几个拼法（reviewListConditions、bounceFilterDimensions、
// deliveryScopeConditions）组出来，钉的是页面真会发出去的那一串。

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

// Covers: 运输履约各册——registry 选册；交接结果 verdict、总单 standing 由状态胶囊下推，挡回册的两个筛选框是 booking、source 两维。
test('运输履约各册以 registry 选册，下推的胶囊、筛选框、检索词与游标跟在后面', async () => {
  const urls = captureRequests();
  await listTransportFulfillmentRecords(transportFulfillmentConditions('transport-schedule'));
  await listTransportFulfillmentRecords(
    transportFulfillmentConditions(
      'transport-handover',
      reviewListConditions(' SYN-DPCL ', transportPushedStatus['transport-handover'], 'REFUSED'),
    ),
    'c1',
  );
  await listTransportFulfillmentRecords(
    transportFulfillmentConditions(
      'carrier-master-document',
      reviewListConditions('', transportPushedStatus['carrier-master-document'], 'IN_FORCE'),
    ),
  );
  await listTransportFulfillmentRecords(
    transportFulfillmentConditions(
      'booking-answer-bounce',
      reviewListConditions('', undefined, 'ALL', bounceFilterDimensions({ booking: ' SYN-BK-1 ', source: 'OPERATOR/SYN-x#1' })),
    ),
  );
  // 订舱册的状态由读口派生、不是筛选维：页上选了也不进查询串。
  await listTransportFulfillmentRecords(
    transportFulfillmentConditions('booking-request', reviewListConditions('SYN-BK', transportPushedStatus['booking-request'], '已撤销')),
  );
  deepEqual(urls, [
    '/transport-fulfillment-records?registry=transport-schedule',
    '/transport-fulfillment-records?registry=transport-handover&verdict=REFUSED&q=SYN-DPCL&after=c1',
    '/transport-fulfillment-records?registry=carrier-master-document&standing=IN_FORCE',
    '/transport-fulfillment-records?registry=booking-answer-bounce&booking=SYN-BK-1&source=OPERATOR%2FSYN-x%231',
    '/transport-fulfillment-records?registry=booking-request&q=SYN-BK',
  ]);
});

// Covers: 网点作业三册——registry 选册；收货记录 kind、集包 phase 下推；待识别货物的「为什么认不出」是页面派生，不进查询串。
test('网点作业各册以 registry 选册，下推的胶囊、检索词与游标跟在后面', async () => {
  const urls = captureRequests();
  await listNodeOperationsRecords(nodeOperationsConditions('reception'));
  await listNodeOperationsRecords(
    nodeOperationsConditions('reception', reviewListConditions('', nodePushedStatus.reception, 'PENDING_IDENTIFICATION')),
  );
  await listNodeOperationsRecords(
    nodeOperationsConditions('consolidation-unit', reviewListConditions('SYN-SEAL', nodePushedStatus['consolidation-unit'], 'SEALED')),
    'c1',
  );
  await listNodeOperationsRecords(
    nodeOperationsConditions('unidentified-item', reviewListConditions('SYN-DPCL', nodePushedStatus['unidentified-item'], 'ALL')),
  );
  deepEqual(urls, [
    '/node-operations-records?registry=reception',
    '/node-operations-records?registry=reception&kind=PENDING_IDENTIFICATION',
    '/node-operations-records?registry=consolidation-unit&phase=SEALED&q=SYN-SEAL&after=c1',
    '/node-operations-records?registry=unidentified-item&q=SYN-DPCL',
  ]);
});

// Covers: 按实物列的处置清单走另一本册；待识别册答复里的 units 只是它的第一页，往后翻带游标走这一口。
test('处置清单读 identity-review-unit 那本册，翻页只带游标', async () => {
  const urls = captureRequests();
  await listIdentityReviewUnits();
  await listIdentityReviewUnits('c1');
  deepEqual(urls, [
    '/node-operations-records?registry=identity-review-unit',
    '/node-operations-records?registry=identity-review-unit&after=c1',
  ]);
});

// Covers: 送站两区——节点与单据号是选择器（逐字、去空白），检索框下推成 q；什么都不填即裸路径。
test('送站预报与交接单的节点、单据号走选择器，检索词与游标跟在后面', async () => {
  const urls = captureRequests();
  const blank = { node: '', day: '', documentNumber: '' };
  const preAdvices = deliveryScopeConditions(preAdviceFilterKeys, { node: ' SYN-NODE-01 ', day: '', documentNumber: 'SYN-PA-1' }, 'SYN-CUST');
  const notes = deliveryScopeConditions(handoverNoteFilterKeys, { ...blank, node: 'SYN-NODE-01' }, ' SYN-DRIVER ');
  if (preAdvices === null || notes === null) throw new Error('不带日子的条件不该答 null');
  await listDeliveryPreAdvices(deliveryScopeConditions(preAdviceFilterKeys, blank) ?? {});
  await listDeliveryPreAdvices(preAdvices, 'c1');
  await listDeliveryHandoverNotes(notes);
  deepEqual(urls, [
    '/node-operations-delivery-pre-advices',
    '/node-operations-delivery-pre-advices?documentNumber=SYN-PA-1&node=SYN-NODE-01&q=SYN-CUST&after=c1',
    '/node-operations-delivery-handover-notes?node=SYN-NODE-01&q=SYN-DRIVER',
  ]);
});

// Covers: 外部轨迹——source、view 两个选择器按参数名排；effectiveBasis 胶囊下推；不翻页的视图也带着选择器。
test('外部轨迹以 source、view 选册，确认方式、检索词与游标跟在后面', async () => {
  const urls = captureRequests();
  await listExternalTrackingFacts(externalTrackingFactConditions('SYN-SRC-01', 'pending'));
  await listExternalTrackingFacts(
    externalTrackingFactConditions('SYN-SRC-01', 'current', reviewListConditions('SYN-DPCL', effectiveBasisStatus, 'JUDGED_BY_RULE')),
    'c1',
  );
  deepEqual(urls, [
    '/transport-fulfillment-external-tracking-facts?source=SYN-SRC-01&view=pending',
    '/transport-fulfillment-external-tracking-facts?source=SYN-SRC-01&view=current&effectiveBasis=JUDGED_BY_RULE&q=SYN-DPCL&after=c1',
  ]);
});

// Covers: 答复的 page 原样交出，翻页条据它算页数与下一页。
test('答复的 page 原样交出', async () => {
  captureRequests();
  for (const result of [
    await listTransportFulfillmentRecords(transportFulfillmentConditions('booking-answer-bounce')),
    await listNodeOperationsRecords(nodeOperationsConditions('unidentified-item')),
    await listIdentityReviewUnits(),
    await listDeliveryPreAdvices({}),
    await listDeliveryHandoverNotes({}),
    await listExternalTrackingFacts(externalTrackingFactConditions('SYN-SRC-01', 'current')),
  ]) {
    if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
    deepEqual(result.body.page, page);
  }
});

// Covers: 读口对集外的词、坏游标答 400 并带一句理由散文，原样进 detail。
test('400 带回的理由散文原样进 detail', async () => {
  const detail = '游标与本次的排序或筛选不符，请从第一页重取';
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail } }), { status: 400 })) as typeof fetch;
  const answer = await listTransportFulfillmentRecords(transportFulfillmentConditions('transport-handover'), 'stale');
  deepEqual(answer, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST', detail });
  equal((await listNodeOperationsRecords(nodeOperationsConditions('reception'), 'stale')).kind, 'callerProblem');
});
