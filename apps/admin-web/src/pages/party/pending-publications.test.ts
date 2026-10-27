import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { PublicationDraftRecord } from './publication-draft-api';
import {
  canonicalDocumentText,
  decisionsOf,
  draftDecisions,
  draftFacts,
  draftKeyOf,
  draftSearchText,
  pendingCountSummary,
  pendingRowsOf,
  unreadableRecovery,
} from './pending-publications';

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
    pendingRowsOf({ outcome: 'PUBLICATION_DRAFTS_LISTED', drafts: [first, second] }).map((row) => row.key),
    ['CREDIT_POLICY|CP-B|v2', 'CREDIT_POLICY|CP-A|v1'],
  );
  equal(draftKeyOf(second), 'CREDIT_POLICY|CP-A|v1');
  deepEqual(pendingRowsOf({ outcome: 'PUBLICATION_DRAFTS_LISTED', drafts: [] }), []);
});

// Covers: AT-PC-068——待批准才摆批准、已批准才摆发布、已发布两样都不摆；陌生状态词一样都不摆（宁可不亮）。
test('批准与发布按状态词各亮各的', () => {
  deepEqual(decisionsOf('PENDING_APPROVAL'), { approve: true, publish: false });
  deepEqual(decisionsOf('APPROVED'), { approve: false, publish: true });
  deepEqual(decisionsOf('PUBLISHED'), { approve: false, publish: false });
  deepEqual(decisionsOf('SOMETHING_NEW'), { approve: false, publish: false });
});

// Covers: AT-PC-068——抽屉复述版本壳与指名引用、状态、录入痕迹与完整的两个摘要；批准与发布的痕迹在场才出现。
test('事实带出版本壳、痕迹与完整摘要', () => {
  const pending = draftFacts(record(), 'UTC');
  const labels = pending.map((fact) => fact.label);
  deepEqual(labels, ['类别', '对象', '版本号', '适用范围', '生效起点', '生效止点', '状态', '录入者', '录入时刻', '内容摘要', '壳摘要']);
  equal(pending.find((fact) => fact.label === '状态')?.value, '待批准');
  equal(pending.find((fact) => fact.label === '内容摘要')?.value, 'PCC-1:abc123（算法 PCC-1）');
  equal(pending.find((fact) => fact.label === '生效止点')?.value, '持续有效');

  equal(pending.find((fact) => fact.label === '壳摘要')?.value, 'PCS-1:def456');

  // 指名引用在壳上、不在文档里，抽屉按类别逐条列出。
  const referencing = draftFacts(record({ references: [{ kind: 'PRICE_RULE', objectId: 'PRICE-1' }] }), 'UTC');
  equal(referencing.find((fact) => fact.label === '引用商业价格政策')?.value, 'PRICE-1');

  const approved = draftFacts(
    record({ status: 'APPROVED', approver: 'dex#op-2', approvedAt: '2026-10-26T09:00:00Z' }),
    'UTC',
  );
  equal(approved.find((fact) => fact.label === '批准者')?.value, 'dex#op-2');
  equal(approved.some((fact) => fact.label === '批准时刻'), true);
  equal(approved.some((fact) => fact.label === '发布时刻'), false);
});

// Covers: AT-PC-068——规范化文档只排版；文档不在场（列表行）是空串，不是 "undefined"。
test('规范化文档缩进排版，缺席为空串', () => {
  equal(canonicalDocumentText({ a: 1, b: ['x'] }), '{\n  "a": 1,\n  "b": [\n    "x"\n  ]\n}');
  equal(canonicalDocumentText(undefined), '');
});

// Covers: AT-PC-068——检索命中类别中文名、对象、版本、范围、状态词与录入者；大小写不分。
test('检索串覆盖各列', () => {
  const text = draftSearchText(record({ approver: 'dex#op-2', status: 'APPROVED' }), '信用政策');
  for (const needle of ['信用政策', 'cp-syn-1', 'v2', 'syn-scope-01', '已批准', 'dex#op-1', 'dex#op-2']) {
    equal(text.includes(needle), true, needle);
  }
});

// Covers: AT-PC-068——计数摘要分状态数。
test('计数摘要分待批准与已批准', () => {
  equal(
    pendingCountSummary([record(), record({ objectId: 'B' }), record({ objectId: 'C', status: 'APPROVED' })]),
    '当前返回 3 份尚未发布的载体（待批准 2，已批准 1）',
  );
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
  equal(unreadableRecovery('APPROVED').includes('换新的版本号'), true);
  equal(unreadableRecovery('PUBLISHED').includes('不需要处理'), true);
  equal(unreadableRecovery('SOMETHING_NEW'), '');
});

// Covers: AT-PC-071——计数摘要把读不回的另报；搜「读不回」能找到它们；没有读不回的行时摘要与从前同形。
test('计数摘要另报读不回的，检索认得读不回', () => {
  const records = [record(), record({ objectId: 'CP-2', readable: false, canonicalization: 'PCC-9', contentDigest: 'PCC-9:x' })];
  equal(pendingCountSummary(records), '当前返回 2 份尚未发布的载体（待批准 2，已批准 0，其中 1 份本构建读不回）');
  equal(pendingCountSummary([record()]), '当前返回 1 份尚未发布的载体（待批准 1，已批准 0）');
  equal(draftSearchText(records[1], '信用政策').includes('读不回'), true);
  equal(draftSearchText(records[0], '信用政策').includes('读不回'), false);
});
