// 「待批准发布」页的纯逻辑（ADR-0314 决定三）：把列表读口交回的载体折成行、给详情抽屉复述的几项、批准与发布各在哪个状态
// 下可做。全部是纯函数，node:test 钉着；组件 PendingPublicationsPage.tsx / PendingDraftDrawer.tsx 只负责摆。
//
// **本文件不算摘要、不判谁能批准**：摘要原样取自服务端的读回，批准职责由服务端的审批职责规则判。这里只按状态词决定按钮
// 亮不亮——状态词是服务端的，亮了按下去服务端仍会答它的结果。

import { labelOf } from './presentation';
import { momentText, publicationKindLabels, publicationSummary, type PublicationFact } from './publication-summary';
import { draftStatusLabels, type PublicationDraftListResponseBody, type PublicationDraftRecord } from './publication-draft-api';

/** 表里的一行：键是（类别 × 对象 × 版本）三元，与读回路径同一个身份。 */
export interface PendingDraftRow {
  key: string;
  source: PublicationDraftRecord;
}

export function draftKeyOf(record: Pick<PublicationDraftRecord, 'kind' | 'objectId' | 'version'>): string {
  return `${record.kind}|${record.objectId}|${record.version}`;
}

/** 列表答复的行，保服务端给的顺序（录入时刻由新到旧）。 */
export function pendingRowsOf(body: PublicationDraftListResponseBody): PendingDraftRow[] {
  return body.drafts.map((source) => ({ key: draftKeyOf(source), source }));
}

/** 批准与发布各在哪个状态下摆出：待批准才能批准，已批准才能发布，已发布两样都不再有。 */
export interface DraftDecisions {
  approve: boolean;
  publish: boolean;
}

export function decisionsOf(status: string): DraftDecisions {
  return { approve: status === 'PENDING_APPROVAL', publish: status === 'APPROVED' };
}

/**
 * 抽屉里复述的事实：版本壳（与发布前的「将要发布」同一份），加录入与批准的痕迹，加完整的内容摘要——批准者拿它去对。
 * 录入者与批准者是操作者主体引用原串，原样示出。
 */
export function draftFacts(record: PublicationDraftRecord, timeZone: string): PublicationFact[] {
  const facts = publicationSummary(record, timeZone);
  // 指名引用在壳上、不在规范化文档里：批准的是它们连同范围与区间一起，抽屉里要看得见。
  for (const reference of record.references) {
    facts.push({
      label: `引用${publicationKindLabels[reference.kind] ?? reference.kind}`,
      value: reference.objectId,
    });
  }
  facts.push(
    { label: '状态', value: labelOf(draftStatusLabels, record.status) },
    { label: '录入者', value: record.submitter },
    { label: '录入时刻', value: momentText(record.submittedAt, timeZone) },
  );
  if (record.approver) facts.push({ label: '批准者', value: record.approver });
  if (record.approvedAt) facts.push({ label: '批准时刻', value: momentText(record.approvedAt, timeZone) });
  if (record.publishedAt) facts.push({ label: '发布时刻', value: momentText(record.publishedAt, timeZone) });
  facts.push(
    { label: '内容摘要', value: `${record.contentDigest}（算法 ${record.canonicalization}）` },
    { label: '壳摘要', value: record.shellDigest },
  );
  return facts;
}

/** 规范化文档的缩进文本。文档是服务端按册折出的、摘要盖住的那一份；这里只排版，键序沿用解析后的对象，值不动。 */
export function canonicalDocumentText(content: unknown): string {
  return content === undefined ? '' : JSON.stringify(content, null, 2);
}

/** 检索用的一串：各格拼起来小写比较，搜类别、对象、版本、范围、状态与录入者都能命中。 */
export function draftSearchText(record: PublicationDraftRecord, kindLabel: string): string {
  return [
    kindLabel,
    record.kind,
    record.objectId,
    record.version,
    record.scope,
    labelOf(draftStatusLabels, record.status),
    record.submitter,
    record.approver ?? '',
  ]
    .join(' ')
    .toLowerCase();
}

/** 计数摘要：只在拿到业务答案后用；分状态数，让批准者一眼看出多少在等。 */
export function pendingCountSummary(records: readonly PublicationDraftRecord[]): string {
  const pending = records.filter((record) => record.status === 'PENDING_APPROVAL').length;
  const approved = records.filter((record) => record.status === 'APPROVED').length;
  return `当前返回 ${records.length} 份尚未发布的载体（待批准 ${pending}，已批准 ${approved}）`;
}
