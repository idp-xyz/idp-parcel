// 运单工作台的筛选、排序与计数纯逻辑（改版第一步，设计源「五、改版示意 · 运单」）。全部是纯函数，node:test 钉着；
// 页面只负责摆。几件事都只在**已取回的行**上做：读口一页答完，不下推成查询参数——那要改端点契约。
//
// 列上的词按用语词典：行是领域的「委托」，界面上叫运单；来源请求键叫客户单号；客户账户在列上显名称，名称从参与方册的
// 客户账户转写（与参与方页同一做法），转写不到时显编码——不补占位文本冒充名称。

import type { WorkbenchSort } from '../../templates/workbench';
import { byInstant, byString, matchesSearch } from '../party/list-order';
import type { CustomerAccountListResponseBody } from '../party/api';
import type { ApiResult as CatalogueApiResult } from '../catalogue-api';
import { labelFor } from '../../vocabulary/status';
import type { ShipmentRequestSummary } from './api';

/** 客户账户编码 → 客户名称。只收转写得到名称的账户；查不到的编码由页面原样显编码。 */
export type CustomerNameIndex = ReadonlyMap<string, string>;

/**
 * 从客户账户册的一页答复建名称索引；非业务答案交回 null（页面按「名称暂不可用」显编码，不把一次读不到当成「册上没有」）。
 * 一个账户在册上只有最新修订一行，同一编码出现两次时后一行覆盖前一行——两行名称相同，覆盖无损。
 */
export function customerNameIndexOf(answer: CatalogueApiResult<CustomerAccountListResponseBody> | null): Map<string, string> | null {
  if (answer?.kind !== 'outcome') return null;
  const index = new Map<string, string>();
  for (const account of answer.body.accounts) {
    if (account.customerPartyNameKnown && account.customerPartyName) index.set(account.accountId, account.customerPartyName);
  }
  return index;
}

/** 列上「客户」那一格的字：有名称显名称，没有显编码。 */
export function customerDisplayOf(row: ShipmentRequestSummary, names: CustomerNameIndex): { text: string; isCode: boolean } {
  const name = names.get(row.customerAccountId);
  return name !== undefined ? { text: name, isCode: false } : { text: row.customerAccountId, isCode: true };
}

/** 状态筛选：「全部」或读模型上出现过的某个状态码。 */
export type ShipmentStateFilter = 'ALL' | string;

export interface ShipmentListFilter {
  search: string;
  state: ShipmentStateFilter;
}

/** 搜索是包含匹配、不分大小写，命中运单号 / 客户单号 / 客户编码 / 客户名称任一格。 */
export function filterShipments(
  rows: readonly ShipmentRequestSummary[],
  filter: ShipmentListFilter,
  names: CustomerNameIndex,
): ShipmentRequestSummary[] {
  return rows.filter((row) => {
    if (filter.state !== 'ALL' && row.state !== filter.state) return false;
    return matchesSearch(filter.search, [
      row.shipmentRequestId,
      row.sourceRequestKey,
      row.customerAccountId,
      names.get(row.customerAccountId),
    ]);
  });
}

/** 表头可排的四列。 */
export type ShipmentSortKey = 'shipment-id' | 'customer' | 'parcel-count' | 'submitted-at';

/** 默认提交时间新→旧：运营与客服最常看「刚进来的那一张」。 */
export const defaultShipmentSort: WorkbenchSort = { key: 'submitted-at', dir: -1 };

/** 按表头排序交回新数组，不改输入；同值保持原序。认不得的键原样交回。客户列按名称排，没有名称的按编码。 */
export function sortShipments(
  rows: readonly ShipmentRequestSummary[],
  sort: WorkbenchSort,
  names: CustomerNameIndex,
): ShipmentRequestSummary[] {
  const compare = comparatorOf(sort.key as ShipmentSortKey, names);
  if (compare === null) return [...rows];
  return [...rows].sort((left, right) => sort.dir * compare(left, right));
}

function comparatorOf(
  key: ShipmentSortKey,
  names: CustomerNameIndex,
): ((left: ShipmentRequestSummary, right: ShipmentRequestSummary) => number) | null {
  switch (key) {
    case 'shipment-id':
      return (left, right) => byString(left.shipmentRequestId, right.shipmentRequestId);
    case 'customer':
      return (left, right) => byString(customerDisplayOf(left, names).text, customerDisplayOf(right, names).text);
    case 'parcel-count':
      return (left, right) => left.declaredParcelCount - right.declaredParcelCount;
    case 'submitted-at':
      return (left, right) => byInstant(left.submittedAt, right.submittedAt);
    default:
      return null;
  }
}

/** 一个状态码在这一页上出现的次数，按运单生命周期的自然顺序排：待受理 → 已受理 → 已拒绝 → 已撤回，读模型上别的码接在后面。 */
export interface ShipmentStateCount {
  code: string;
  label: string;
  count: number;
}

const stateOrder = ['SUBMITTED', 'ACCEPTED', 'REJECTED', 'WITHDRAWN'];

/**
 * 命令头指标与状态胶囊的计数，数的是已取回的这一页；只列真出现过的状态——指标即筛，一格 0 的筛选没有意义。
 * 词经用语词典译，不在这里另抄一份。
 */
export function countShipmentsByState(rows: readonly ShipmentRequestSummary[]): ShipmentStateCount[] {
  const counts = new Map<string, number>();
  for (const row of rows) counts.set(row.state, (counts.get(row.state) ?? 0) + 1);
  const codes = [...counts.keys()].sort((left, right) => {
    const leftIndex = stateOrder.indexOf(left);
    const rightIndex = stateOrder.indexOf(right);
    const leftRank = leftIndex === -1 ? stateOrder.length : leftIndex;
    const rightRank = rightIndex === -1 ? stateOrder.length : rightIndex;
    return leftRank - rightRank || byString(left, right);
  });
  return codes.map((code) => ({ code, label: labelFor(code), count: counts.get(code) ?? 0 }));
}

/** 工具条右端的计数摘要：总数与当前显示数分开报，筛空时照显「共 N 张，当前显示 0 张」，表格区另显 noMatch 那一句。 */
export function shipmentCountSummary(total: number, visible: number): string {
  return `共 ${total} 张运单，当前显示 ${visible} 张`;
}

/** 筛出为空时表格区那一行的话；点明是「筛选条件」，与空态「还没有运单」分得开。 */
export const shipmentNoMatchNote = '当前筛选条件下没有匹配的运单';
