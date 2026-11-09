// 「审批与发布」页的纯逻辑（ADR-0314 决定三）：把列表读口交回的载体折成行、给详情抽屉复述的几项、批准与发布各在哪个状态
// 下可做、胶囊筛的查询条件。全部是纯函数，node:test 钉着；组件 PendingPublicationsPage.tsx / PendingDraftPanel.tsx 只负责摆。
//
// **本文件不算摘要、不判谁能批准**：摘要原样取自服务端的读回，批准职责由服务端的审批职责规则判。这里只按状态词决定按钮
// 亮不亮——状态词是服务端的，亮了按下去服务端仍会答它的结果。

import type { TechnicalRow } from '../../components/TechnicalDetails';
import { shortPrincipalRef } from '../../components/principal-ref';
import { labelFor } from '../../vocabulary/status';
import type { CatalogueConditions } from '../catalogue-query';
import { momentText, publicationKindLabels, publicationSummary, type PublicationFact } from './publication-summary';
import type { PublicationDraftListResponseBody, PublicationDraftRecord } from './publication-draft-api';

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

/** 读不出内容的改动（本版系统折不回它的正文）没有可批准或发布的内容：两个动作都不摆，其余照状态。 */
export function draftDecisions(record: Pick<PublicationDraftRecord, 'status' | 'readable'>): DraftDecisions {
  return record.readable ? decisionsOf(record.status) : { approve: false, publish: false };
}

/**
 * 读不出内容的改动按状态各自的恢复动作。读不出只有两类成因（内容格式是本版系统不认得的，或含本版系统已不再接受的内容），
 * 新录入的摘要必与读不出那一行的不同，所以`待批准`的行可被同一版本号的重新录入替换；`已批准`的内容已固定，只能换版本号；
 * `已发布`的版本已在册，不需要动作。陌生状态词不替服务端猜动作，只说读不出。
 */
export function unreadableRecovery(status: string): string {
  switch (status) {
    case 'PENDING_APPROVAL':
      return '请在原来的页面按同一版本号重新录入一次，新内容会替换这一份，之后照常批准。';
    case 'APPROVED':
      return '已批准的内容不能再改；要发布，请换一个新的版本号重新录入、批准、发布。';
    case 'PUBLISHED':
      return '这一版已经发布，已生效的内容不受影响，不需要处理。';
    default:
      return '';
  }
}

/**
 * 详情里给人核对的事实：改的是什么（类别、编号、版本、生效期），谁提交、谁批准、各在什么时候。适用范围引用、两个摘要、
 * 主体引用全串与状态原码只有排查与对接时才用，归 draftTechnicalRows 进「技术详情」——批准者核对的是内容本身。
 */
export function draftFacts(record: PublicationDraftRecord, timeZone: string): PublicationFact[] {
  const facts = publicationSummary(record, timeZone).filter((fact) => fact.label !== '适用范围');
  // 指名引用在壳上、不在内容里：批准的是它们连同生效期一起，详情里要看得见。
  for (const reference of record.references) {
    facts.push({
      label: `引用${publicationKindLabels[reference.kind] ?? reference.kind}`,
      value: reference.objectId,
    });
  }
  facts.push(
    { label: '状态', value: labelFor(record.status) },
    { label: '提交人', value: shortPrincipalRef(record.submitter) },
    { label: '提交时间', value: momentText(record.submittedAt, timeZone) },
  );
  if (record.approver) facts.push({ label: '批准人', value: shortPrincipalRef(record.approver) });
  if (record.approvedAt) facts.push({ label: '批准时间', value: momentText(record.approvedAt, timeZone) });
  if (record.publishedAt) facts.push({ label: '发布时间', value: momentText(record.publishedAt, timeZone) });
  return facts;
}

/** 「技术详情」折叠区的几行：摘要整串不截断——它就是批准与发布时带上去的那一份，对日志的人要拿它去对。 */
export function draftTechnicalRows(record: PublicationDraftRecord): TechnicalRow[] {
  return [
    { label: '适用范围', value: record.scope },
    { label: '状态码', value: record.status },
    { label: '提交人', value: record.submitter },
    { label: '批准人', value: record.approver ?? '' },
    { label: '内容摘要', value: record.contentDigest },
    { label: '摘要算法', value: record.canonicalization },
    { label: '版本壳摘要', value: record.shellDigest },
    { label: '读不出的原因', value: record.unreadableCause ?? '' },
  ];
}

/** 提交内容的缩进文本。内容是服务端按册折出的、摘要盖住的那一份；这里只排版，键序沿用解析后的对象，值不动。 */
export function canonicalDocumentText(content: unknown): string {
  return content === undefined ? '' : JSON.stringify(content, null, 2);
}

/** 读不出的行在筛选里叫的那个词；列表与详情同一个说法。 */
export const unreadableWord = '内容读不出';

/**
 * 胶囊筛：全部、按状态码，或只看读不出的。读不出的单列一格，免得混在「待批准」里被当成能批的。
 * 按状态的两格下推成读口的 status 维（ADR-0144）；读不出不是库上的一列——可读与否经重建门之后才知道（ADR-0344）——读口筛不了，
 * 那一格只筛到手的这一页（pendingRowsOnPage）。
 */
export type PendingStatusFilter = 'ALL' | 'PENDING_APPROVAL' | 'APPROVED' | 'UNREADABLE';

/** 待批准清单一问的条件：检索词与状态（「全部」与「读不出」都不带这一维）；录入时刻新→旧是读口的缺省序，不写进查询串。 */
export function pendingListConditions(keyword: string, filter: PendingStatusFilter): CatalogueConditions {
  return { q: keyword, filters: { status: filter === 'PENDING_APPROVAL' || filter === 'APPROVED' ? [filter] : [] } };
}

/** 这一页上摆哪几行：按状态的两格读口已筛过，这里只做读不出那一格。 */
export function pendingRowsOnPage(rows: readonly PendingDraftRow[], filter: PendingStatusFilter): PendingDraftRow[] {
  return filter === 'UNREADABLE' ? rows.filter((row) => !row.source.readable) : [...rows];
}
