// 运单工作台的纯逻辑（改版第一步，设计源「五、改版示意 · 运单」）。全部是纯函数，node:test 钉着；页面只负责摆。
// 运单列表读口已迁到 ADR-0144：检索词、状态与提交时间的先后都下推成查询条件（shipmentListConditions），不在答回来的那一页上
// 再筛、再排、再数——截断之后再筛，答出的「没有」可能是假的。
//
// 列上的词按用语词典：行是领域的「委托」，界面上叫运单；来源请求键叫客户单号；客户账户在列上显名称，名称从参与方册的
// 客户账户转写（与参与方页同一做法），转写不到时显编码——不补占位文本冒充名称。

import type { WorkbenchSort } from '../../templates/workbench';
import type { CustomerAccountListResponseBody } from '../party/api';
import type { ApiResult as CatalogueApiResult } from '../catalogue-api';
import type { CatalogueConditions } from '../catalogue-query';
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

/**
 * 状态维的封闭词表，与 ports.ShipmentRequestViewPages 里 state 那一维一一对应——读口按它校验值，集外的答 400。按运单生命周期排：
 * 待受理 → 已受理 → 已拒绝 → 已撤回；状态胶囊照这个次序摆。
 */
export const shipmentStates = ['SUBMITTED', 'ACCEPTED', 'REJECTED', 'WITHDRAWN'] as const;

/** 状态筛选：「全部」或状态维词表里的一个码。 */
export type ShipmentStateFilter = 'ALL' | (typeof shipmentStates)[number];

/** 表头可排的只有提交时间一列，默认新→旧：运营与客服最常看「刚进来的那一张」。 */
export const defaultShipmentSort: WorkbenchSort = { key: 'submitted-at', dir: -1 };

/**
 * 屏上的检索词、状态胶囊与表头先后 → 读口的查询条件。新→旧就是读口的缺省序，不写进查询串：不带检索与状态的第一页是裸路径。
 * 读口能排的另一维是委托标识，而运单号那一列显的是单据号，按委托标识排对不上屏上的次序，所以运单号列不给排。
 */
export function shipmentListConditions(keyword: string, state: ShipmentStateFilter, sort: WorkbenchSort): CatalogueConditions {
  return {
    q: keyword,
    filters: state === 'ALL' ? {} : { state: [state] },
    sort: sort.key === defaultShipmentSort.key && sort.dir === 1 ? 'submittedAt' : undefined,
  };
}

/** 带检索词或状态答出零行时表格区那一行的话；点明是「筛选条件」，与空态「还没有运单」分得开。 */
export const shipmentNoMatchNote = '当前筛选条件下没有匹配的运单';
