// 对账单页的页内位置：哪一签、客户对账单签是不是只看挂着未了结异议的，放进地址的查询串（`#/reconciliation?tab=…&only=…`），首页
// 「今日待办」点进来就落在筛好的那一签；刷新与换标签回来也还在。纯函数、只引零依赖的 address-query，node:test 直接跑。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

export const RECONCILIATION_TAB_PARAM = 'tab';
/** 只看哪一种：与保存视图的 `?view=`、检索词的 `?q=` 不同键。 */
export const RECONCILIATION_ONLY_PARAM = 'only';

export type ReconciliationTab = 'customer' | 'supplier';

const DISPUTED = 'disputed';

export function reconciliationTabOf(hash: string): ReconciliationTab {
  return hashQueryValue(hash, RECONCILIATION_TAB_PARAM) === 'supplier' ? 'supplier' : 'customer';
}

/** 客户对账单签是否只看挂着未了结异议的单（至少一项异议还没有终局结果）。 */
export function disputedOnlyOf(hash: string): boolean {
  return hashQueryValue(hash, RECONCILIATION_ONLY_PARAM) === DISPUTED;
}

/** 把签与「只看待对账」写回地址；取缺省值的那一项不写。只看争议只在客户对账单签上有意义，换到供应商签就清掉。 */
export function reconciliationHash(hash: string, place: { tab: ReconciliationTab; disputedOnly: boolean }): string {
  const withTab = hashWithQueryValue(hash, RECONCILIATION_TAB_PARAM, place.tab === 'customer' ? '' : place.tab);
  return hashWithQueryValue(withTab, RECONCILIATION_ONLY_PARAM, place.tab === 'customer' && place.disputedOnly ? DISPUTED : '');
}

/** 首页「今日待办」待对账那一格点进来的地址。 */
export const disputedStatementsHash = reconciliationHash('#/reconciliation', { tab: 'customer', disputedOnly: true });
