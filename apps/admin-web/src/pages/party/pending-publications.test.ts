import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { PublicationDraftRecord } from './publication-draft-api';
import {
  canonicalDocumentText,
  decisionsOf,
  draftDecisions,
  draftFacts,
  draftKeyOf,
  draftTechnicalRows,
  pendingListConditions,
  pendingRowsOf,
  pendingRowsOnPage,
  unreadableRecovery,
} from './pending-publications';
import { catalogueRequestPath } from '../catalogue-query';

const firstPage = { size: 200, next: null, total: null };

// 本文件钉的是「待批准发布」页的纯逻辑（ADR-0314 决定三）：行的身份、按状态摆出的动作、抽屉复述的事实。
// 不算摘要、不判谁能批准——摘要原样取自读回，批准职责由服务端判。

function record(over: Partial<PublicationDraftRecord> = {}): PublicationDraftRecord {
  return {
    kind: 'CREDIT_POLICY',
    objectId: 'CP-SYN-1',
    version: 'v2',
    scope: 'SYN-SCOPE-01',
    effectiveStartsAt: '2026-11-01T00:00:00Z',
    references: [],
    canonicalization: 'PCC-1',
    contentDigest: 'PCC-1:abc123',
    shellDigest: 'PCS-1:def456',
    submitter: 'dex#op-1',
    submittedAt: '2026-10-26T08:00:00Z',
    status: 'PENDING_APPROVAL',
    readable: true,
    ...over,
  };
}

// Covers: AT-PC-068——行的身份是（类别 × 对象 × 版本）三元，顺序照服务端给的（录入时刻由新到旧），空列表是空行。
test('行键取版本身份三元，顺序保服务端给的', () => {
  const first = record({ objectId: 'CP-B' });
  const second = record({ objectId: 'CP-A', version: 'v1' });
  deepEqual(
    pendingRowsOf({ outcome: 'PUBLICATION_DRAFTS_LISTED', drafts: [first, second], page: firstPage }).map((row) => row.key),
    ['CREDIT_POLICY|CP-B|v2', 'CREDIT_POLICY|CP-A|v1'],
  );
  equal(draftKeyOf(second), 'CREDIT_POLICY|CP-A|v1');
  deepEqual(pendingRowsOf({ outcome: 'PUBLICATION_DRAFTS_LISTED', drafts: [], page: firstPage }), []);
});

// Covers: AT-PC-068——待批准才摆批准、已批准才摆发布、已发布两样都不摆；陌生状态词一样都不摆（宁可不亮）。
test('批准与发布按状态词各亮各的', () => {
  deepEqual(decisionsOf('PENDING_APPROVAL'), { approve: true, publish: false });
  deepEqual(decisionsOf('APPROVED'), { approve: false, publish: true });
  deepEqual(decisionsOf('PUBLISHED'), { approve: false, publish: false });
  deepEqual(decisionsOf('SOMETHING_NEW'), { approve: false, publish: false });
});

// Covers: AT-PC-068——详情复述版本壳与指名引用、状态、提交痕迹；批准与发布的痕迹在场才出现。摘要、范围引用与主体全串
// 进技术详情，不在核对用的事实里。
test('事实带出版本壳与痕迹，摘要与引用进技术详情', () => {
  const pending = draftFacts(record(), 'UTC');
  const labels = pending.map((fact) => fact.label);
  deepEqual(labels, ['类别', '对象', '版本号', '生效起点', '生效止点', '状态', '提交人', '提交时间']);
  equal(pending.find((fact) => fact.label === '状态')?.value, '待批准');
  equal(pending.find((fact) => fact.label === '生效止点')?.value, '持续有效');
  equal(pending.find((fact) => fact.label === '提交人')?.value, 'op-1');

  const technical = draftTechnicalRows(record());
  equal(technical.find((row) => row.label === '内容摘要')?.value, 'PCC-1:abc123');
  equal(technical.find((row) => row.label === '版本壳摘要')?.value, 'PCS-1:def456');
  equal(technical.find((row) => row.label === '适用范围')?.value, 'SYN-SCOPE-01');
  equal(technical.find((row) => row.label === '提交人')?.value, 'dex#op-1');
  equal(technical.find((row) => row.label === '状态码')?.value, 'PENDING_APPROVAL');

  // 指名引用在壳上、不在文档里，详情按类别逐条列出。
  const referencing = draftFacts(record({ references: [{ kind: 'PRICE_RULE', objectId: 'PRICE-1' }] }), 'UTC');
  equal(referencing.find((fact) => fact.label === '引用商业价格政策')?.value, 'PRICE-1');

  const approved = draftFacts(
    record({ status: 'APPROVED', approver: 'dex#op-2', approvedAt: '2026-10-26T09:00:00Z' }),
    'UTC',
  );
  equal(approved.find((fact) => fact.label === '批准人')?.value, 'op-2');
  equal(approved.some((fact) => fact.label === '批准时间'), true);
  equal(approved.some((fact) => fact.label === '发布时间'), false);
});

// Covers: AT-PC-068——规范化文档只排版；文档不在场（列表行）是空串，不是 "undefined"。
test('规范化文档缩进排版，缺席为空串', () => {
  equal(canonicalDocumentText({ a: 1, b: ['x'] }), '{\n  "a": 1,\n  "b": [\n    "x"\n  ]\n}');
  equal(canonicalDocumentText(undefined), '');
});

// 检索词与按状态的两格下推到读口（ADR-0144）；「全部」与「读不出」都不带 status 维——读不出不是库上的一列。
test('检索词与状态下推成查询串，读不出不下推', () => {
  const path = (filter: Parameters<typeof pendingListConditions>[1]) =>
    catalogueRequestPath('/commercial-open-publication-drafts', pendingListConditions(' SYN-CP ', filter), null);
  equal(path('ALL'), '/commercial-open-publication-drafts?q=SYN-CP');
  equal(path('PENDING_APPROVAL'), '/commercial-open-publication-drafts?status=PENDING_APPROVAL&q=SYN-CP');
  equal(path('APPROVED'), '/commercial-open-publication-drafts?status=APPROVED&q=SYN-CP');
  equal(path('UNREADABLE'), '/commercial-open-publication-drafts?q=SYN-CP');
});

// Covers: AT-PC-071——本构建读不回的载体（readable 为假）照样是列表里的一行，动作一个都不摆：待批准不摆批准、已批准不摆发布；
// 读得回的照状态。
test('读不回的载体不摆批准与发布，读得回的照状态', () => {
  deepEqual(draftDecisions(record({ readable: false })), { approve: false, publish: false });
  deepEqual(draftDecisions(record({ readable: false, status: 'APPROVED' })), { approve: false, publish: false });
  deepEqual(draftDecisions(record()), { approve: true, publish: false });
  deepEqual(draftDecisions(record({ status: 'APPROVED' })), { approve: false, publish: true });
});

// Covers: AT-PC-071——恢复动作随状态：待批准的按同一版本号重录，已批准的换版本号，已发布的无需处理；陌生状态词不替服务端猜。
test('读不回的恢复动作按状态说，陌生状态不猜', () => {
  equal(unreadableRecovery('PENDING_APPROVAL').includes('同一版本号重新录入'), true);
  equal(unreadableRecovery('APPROVED').includes('新的版本号'), true);
  equal(unreadableRecovery('PUBLISHED').includes('不需要处理'), true);
  equal(unreadableRecovery('SOMETHING_NEW'), '');
});

// Covers: AT-PC-071——读不出的那一格只筛到手的这一页；按状态的两格读口已筛过，页上不再筛一遍。
test('读不出只筛本页，按状态的两格照读口答的摆', () => {
  const rows = pendingRowsOf({
    outcome: 'PUBLICATION_DRAFTS_LISTED',
    drafts: [record(), record({ objectId: 'CP-2', readable: false }), record({ objectId: 'CP-3', status: 'APPROVED' })],
    page: firstPage,
  });
  deepEqual(
    pendingRowsOnPage(rows, 'UNREADABLE').map((row) => row.source.objectId),
    ['CP-2'],
  );
  equal(pendingRowsOnPage(rows, 'ALL').length, 3);
  equal(pendingRowsOnPage(rows, 'PENDING_APPROVAL').length, 3);
});
