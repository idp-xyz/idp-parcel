// 结算各列表页的查询条件与只作用于本页的几句话（票 catalogue-read-pagination-rollout/14；契约 ADR-0144 决定三、四）：检索框与
// 表头先后换成读口的条件，读口点名的维下推，不在答回来的那一页上再筛再排。各册点名了哪些维只在 internal/settlementaccounting/ports
// 的 catalogue_pages.go 一处，这里照它取名、不另造——发了没点名的键，整个请求答 400。
//
// 读口检索不覆盖的（往来单位与公司主体的名称、码的中文词、核销目标）和读口不认的派生状态（账单走到哪一步、应付有没有单号）
// 只能在到手的这一页上筛：保留成只筛本页的控件，页面字面写明「本页」。纯函数，node:test 钉着；页面只负责摆。

import type { WorkbenchSort } from '../../templates/workbench';
import type { CatalogueConditions } from '../catalogue-query';
import { workbenchSortParam } from '../visibility/list-conditions';
import { registryConditions, type CustomerStatementRecord, type RegistryConditions } from './api';

// —— 客户对账单（ports.CustomerStatementPages 与 DisputedCustomerStatementPages）——

/**
 * 「只看待对账」换接争议队列：另一个读口、另一本游标，不带选择器；不勾时是对账单册，带 registry。两者条件签名不同，切换即回第一页。
 * 「不看已作废的单」落成 voided=false，两个读口都认这一维（作废之前已开的争议照旧把已作废的单留在队列里）；勾不勾同样换签名、回第一页。
 */
export function statementListConditions(disputedOnly: boolean, keyword: string, voidedHidden = false): CatalogueConditions {
  const conditions: Omit<CatalogueConditions, 'selectors'> = { q: keyword, filters: voidedHidden ? { voided: ['false'] } : {} };
  return disputedOnly ? conditions : registryConditions('customer-statement', conditions);
}

/** 条件是对账单册那一问（带 registry）；不是就是争议队列那一问。 */
export function isStatementRegistryConditions(
  conditions: CatalogueConditions,
): conditions is RegistryConditions<'customer-statement'> {
  return conditions.selectors?.registry === 'customer-statement';
}

/**
 * 对账单签的条数那一句。争议队列的张数是读口答的整个队列，不随页、检索词与作废与否变；作废几张只数这一页，已按作废筛掉时不报。
 */
export function statementSummary(
  statements: readonly Pick<CustomerStatementRecord, 'voidBasis'>[],
  disputedCount: number | undefined,
  voidedHidden = false,
): string {
  const voided = statements.filter((row) => row.voidBasis).length;
  const onPage = voidedHidden ? `本页 ${statements.length} 张` : null;
  return disputedCount === undefined
    ? (onPage ?? `本页 ${statements.length} 张 · 已作废 ${voided} 张`)
    : `共 ${disputedCount} 张有未了结异议 · ${onPage ?? `本页 ${statements.length} 张，其中已作废 ${voided} 张`}`;
}

// —— 供应商账单（ports.SupplierBillReceptionPages）——

/** 收到时间新→旧：读口的缺省序。 */
export const defaultBillSort: WorkbenchSort = { key: 'received', dir: -1 };

/** 检索词与表头两列（收到时间、账期）的先后下推。账单号列不给排：账单号可缺、旧行的号在主张标识里，读口不放它作可排维。 */
export function billListConditions(keyword: string, sort: WorkbenchSort): RegistryConditions<'supplier-bill-reception'> {
  return registryConditions('supplier-bill-reception', {
    q: keyword,
    sort: workbenchSortParam(sort, { received: 'receivedAt', period: 'period' }, defaultBillSort),
  });
}

// —— 审核应付（ports.AuditedPayablePages）——

/** 审核时间新→旧：读口的缺省序。 */
export const defaultPayableSort: WorkbenchSort = { key: 'audited', dir: -1 };

/** 检索词与审核时间的先后下推。应付单号列不给排：单号可缺（记无号原因），读口不放它作可排维。 */
export function payableListConditions(keyword: string, sort: WorkbenchSort): CatalogueConditions {
  return { q: keyword, sort: workbenchSortParam(sort, { audited: 'auditedAt' }, defaultPayableSort) };
}

// —— 只作用于本页 ——

/**
 * 只筛本页的状态胶囊列哪几格：这一页上出现过的，按 order 的先后；选中的那一格翻到没有它的页也留着，好点掉。不带计数——一页里数出来的
 * 不是各状态的总数，读口也不按这些派生词计数。
 */
export function pageStatusLabels<Row>(
  rows: readonly Row[],
  labelOf: (row: Row) => string,
  order: readonly string[],
  active: string,
): string[] {
  const present = new Set(rows.map(labelOf));
  return order.filter((label) => present.has(label) || label === active);
}

/** 工作台工具条上只筛本页的那一句：没在页内筛时只提醒状态只筛本页；筛了说这一页几条里剩几条，页外的不在里面。 */
export function pageOnlyNote(pageRows: number, shown: number, filtering: boolean, unit: string): string {
  return filtering ? `只筛本页：本页 ${pageRows} ${unit}里有 ${shown} ${unit}` : '状态只筛本页';
}

/** 只筛本页的条件把行筛空了，与读口答零行是两句话。 */
export function pageEmptyNote(subject: string, pageRows: number): string {
  return pageRows > 0 ? `这一页上没有匹配的${subject}；只筛本页的条件不看其余几页，翻页看看` : `没有匹配的${subject}`;
}
