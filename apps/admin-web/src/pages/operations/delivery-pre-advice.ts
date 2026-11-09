// 送站预报区与送站交接单区共用的筛选写法、送站预报的行转写与两本册读口的问题码（票 ps-external-mark-relations/13「做什么」的「查阅读口」）。
//
// 单独成模块而不写在页面里：最容易错的是「日」——读口只收瞬间，一日从哪一刻起算随看的人所在的时区，页面在这里把日子换成本地零点到次日
// 本地零点的左闭右开区间；换错一格，零点那一份就落进前一日。页面组件要 React 与 DOM，这几格是纯函数就能拿 node:test 钉住。

import { formatInstant } from '../catalogue-view';
import type { CatalogueConditions } from '../catalogue-query';
import { documentNumberAbsenceNote } from '../../vocabulary/document-numbering';
import type { DeliveryPreAdviceRecord, DocumentNumberFields } from './delivery-scope-api';

/** 一本册列表认的那一对业务时刻键：预报是预计送达，交接单是开立。键名与 query_delivery_scope_records.go 同字。 */
export interface DeliveryScopeFilterKeys {
  from: string;
  until: string;
}

export const preAdviceFilterKeys: DeliveryScopeFilterKeys = { from: 'expectedArrivalFrom', until: 'expectedArrivalUntil' };
export const handoverNoteFilterKeys: DeliveryScopeFilterKeys = { from: 'openedFrom', until: 'openedUntil' };

/**
 * 页面上人填的几格：节点、日子（`<input type="date">` 的 YYYY-MM-DD，空即不按日筛）与单据号（逐字找，空即不按单据号筛）。单据号在服务端
 * 按（租户，单据号）找，不在已读出的这一页里找：列表一页只有几十份，旧单据不在页里也要找得到（ADR-0355 决定十一「可按单据号检索」）。
 */
export interface DeliveryScopeListFilter {
  node: string;
  day: string;
  documentNumber?: string;
}

/**
 * 把 YYYY-MM-DD 换成那一日本地零点到次日本地零点（左闭右开）的两个瞬间。认不出的日子（含 2 月 30 日这种 Date 会悄悄进位的）答 null。
 */
export function localDayBoundsOf(day: string): { from: string; until: string } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!match) return null;
  const [year, month, date] = [Number(match[1]), Number(match[2]) - 1, Number(match[3])];
  const start = new Date(year, month, date);
  if (start.getFullYear() !== year || start.getMonth() !== month || start.getDate() !== date) return null;
  return { from: start.toISOString(), until: new Date(year, month, date + 1).toISOString() };
}

/**
 * 拼一问的条件：节点、单据号与日子的两端瞬间是读口的选择器（ports.DeliveryPreAdvicePages、DeliveryHandoverNotePages），只带填了的格，
 * 节点与单据号去两端空白；检索词下推成 `q`；状态胶囊落成的筛选维（list-conditions 的 pushedStatusFilters）原样带上。日子认不出时答 null，
 * 页面不发请求——发半截条件，人会把「没按日筛的列表」当成「这一日的」。
 */
export function deliveryScopeConditions(
  keys: DeliveryScopeFilterKeys,
  filter: DeliveryScopeListFilter,
  keyword = '',
  filters: Readonly<Record<string, readonly string[]>> = {},
): CatalogueConditions | null {
  const selectors: Record<string, string> = {};
  const node = filter.node.trim();
  if (node !== '') selectors.node = node;
  const documentNumber = (filter.documentNumber ?? '').trim();
  if (documentNumber !== '') selectors.documentNumber = documentNumber;
  if (filter.day !== '') {
    const bounds = localDayBoundsOf(filter.day);
    if (!bounds) return null;
    selectors[keys.from] = bounds.from;
    selectors[keys.until] = bounds.until;
  }
  return { selectors, filters, q: keyword };
}

/**
 * 筛选条上填了格没有。这几格在读口是选择器，catalogue-query 的 catalogueConditionsNarrow 不把它们算作收窄；可它们一样圈小了列表，
 * 答出零行说的是「这个网点、这一日没有」，不是册为空。
 */
export function deliveryScopeFilterNarrow(filter: DeliveryScopeListFilter): boolean {
  return filter.node.trim() !== '' || filter.day !== '' || (filter.documentNumber ?? '').trim() !== '';
}

/**
 * 列表主列那一格：有单据号显单据号；没有的照旧显内部标识（ADR-0355 决定十一）——上线前开立的、开立时单据编号时区未配置的单据只有它。
 */
export function documentNumberCellOf(fields: DocumentNumberFields, internalId: string): string {
  return fields.documentNumber ?? internalId;
}

/**
 * 技术详情里「单据号」那一格：有号显号；没有号按单据上记下的原因取单据编号词典的说法（vocabulary/document-numbering 的
 * documentNumberAbsenceNote）——「上线前开立」与「时区未配置」分开说，说法只在词典一处。号码与原因都缺席时这一格不显。
 */
export function documentNumberDetailOf(fields: DocumentNumberFields): string | undefined {
  if (fields.documentNumber) return fields.documentNumber;
  if (fields.documentNumberAbsence) return documentNumberAbsenceNote(fields.documentNumberAbsence);
  return undefined;
}

export interface PreAdviceReviewRow {
  key: string;
  preAdviceId: string;
  values: Readonly<Record<string, string>>;
}

/**
 * 一份预报转写成预报区的一行。「未撤回」不写成「有效」：预报对一次送站算不算数，节点、时窗与号码要在收寄时逐条判（ADR-0212 决定二），
 * 列表上说不出。
 */
export function preAdviceRowOf(record: DeliveryPreAdviceRecord): PreAdviceReviewRow {
  return {
    key: `pre-advice:${record.preAdviceId}`,
    preAdviceId: record.preAdviceId,
    values: {
      documentNumber: documentNumberCellOf(record, record.preAdviceId),
      preAdviceId: record.preAdviceId,
      customerAccount: record.customerAccount,
      node: record.node,
      expectedArrivalAt: formatInstant(record.expectedArrivalAt),
      numbers: `${record.customerParcelNumbers.length} 个`,
      deliveringParty: record.deliveringParty ?? '（没给交付方线索）',
      withdrawal: record.withdrawn
        ? `已撤回${record.withdrawnAt ? ` · ${formatInstant(record.withdrawnAt)}` : ''}`
        : '未撤回',
      formedAt: formatInstant(record.formedAt),
    },
  };
}

/** 两本册读口的 problem+json 码说明。 */
export function deliveryScopeReadProblemNote(code: string): string {
  switch (code) {
    case 'DELIVERY_PRE_ADVICE_NOT_VISIBLE':
      return '找不到这份预报；刷新列表再选。';
    case 'DELIVERY_HANDOVER_NOTE_NOT_VISIBLE':
      return '找不到这张交接单；刷新列表再选。';
    case 'MALFORMED_REQUEST':
      return '筛选条件写得不对（网点、单据号不能只填空格，日期要前早后晚）；改正后再查。';
    case 'METHOD_NOT_ALLOWED':
      return '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。';
    case 'INTAKE_FAILED':
      return '登录信息没能核对通过，这次什么都没有保存；请稍后重试。';
    case 'IDENTITY_DEPENDENCY_UNAVAILABLE':
      return '暂时无法核对登录身份，请稍后重试。';
    case 'NO_ANSWER_FORMED':
      return '系统暂时没能读出记录，请稍后重试；这不是「没有记录」。';
    default:
      return `这次没有成功（错误码 ${code}）；请记下错误码联系系统管理员。`;
  }
}
