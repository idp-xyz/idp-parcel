// 对账单页的页内位置：客户对账单是不是只看挂着未了结异议的，放进地址的查询串（`#/reconciliation?only=…`），首页「今日待办」
// 点进来就落在筛好的那一页；刷新与换标签回来也还在。纯函数、只引零依赖的 address-query，node:test 直接跑。
//
// 地址里只有这一项、没有签：本页只列客户对账单，供应商账单在「供应商账单与应付」页（SupplierBillsPage）。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

/** 只看哪一种：与保存视图的 `?view=`、检索词的 `?q=` 不同键。 */
export const RECONCILIATION_ONLY_PARAM = 'only';

const DISPUTED = 'disputed';

/** 是否只看挂着未了结异议的单（至少一项异议还没有终局结果）。 */
export function disputedOnlyOf(hash: string): boolean {
  return hashQueryValue(hash, RECONCILIATION_ONLY_PARAM) === DISPUTED;
}

/** 把「只看待对账」写回地址；不只看时不写这一项，别的键原样。 */
export function reconciliationHash(hash: string, disputedOnly: boolean): string {
  return hashWithQueryValue(hash, RECONCILIATION_ONLY_PARAM, disputedOnly ? DISPUTED : '');
}

/** 首页「今日待办」待对账那一格点进来的地址。 */
export const disputedStatementsHash = reconciliationHash('#/reconciliation', true);
