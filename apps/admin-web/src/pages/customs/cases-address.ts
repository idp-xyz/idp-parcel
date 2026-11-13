// 清关单页的页内位置：哪一签、放行记录签是不是只看卡关的，放进地址的查询串（`#/customs-cases?tab=…&only=…`），首页
// 「今日待办」点进来就落在筛好的那一签；刷新与换标签回来也还在。纯函数、只引零依赖的 address-query，node:test 直接跑。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

export const CASES_TAB_PARAM = 'tab';
/** 只看哪一种：与保存视图的 `?view=`、检索词的 `?q=` 不同键。 */
export const CASES_ONLY_PARAM = 'only';

export const casesTabs = [
  'cases',
  'units',
  'snapshots',
  'submissions',
  'results',
  'manifests',
  'admission',
  'closure-obligations',
  'preconditions',
  'credentials',
  'register',
  'decisions',
] as const;

export type CasesTab = (typeof casesTabs)[number];

const UNADMITTED = 'unadmitted';

export function casesTabOf(hash: string): CasesTab {
  const tab = hashQueryValue(hash, CASES_TAB_PARAM);
  return (casesTabs as readonly string[]).includes(tab) ? (tab as CasesTab) : 'cases';
}

/** 放行记录签是否只看卡关的对象（每个对象最近那一刻的放行检查有一份没过门的）。 */
export function unadmittedOnlyOf(hash: string): boolean {
  return hashQueryValue(hash, CASES_ONLY_PARAM) === UNADMITTED;
}

/** 把签与「只看卡关」写回地址；取缺省值的那一项不写。只看卡关只在放行记录签上有意义，换到别的签就清掉。 */
export function casesHash(hash: string, place: { tab: CasesTab; unadmittedOnly: boolean }): string {
  const withTab = hashWithQueryValue(hash, CASES_TAB_PARAM, place.tab === 'cases' ? '' : place.tab);
  return hashWithQueryValue(withTab, CASES_ONLY_PARAM, place.tab === 'admission' && place.unadmittedOnly ? UNADMITTED : '');
}

/** 首页「今日待办」卡关那一格点进来的地址。 */
export const unadmittedObjectsHash = casesHash('#/customs-cases', { tab: 'admission', unadmittedOnly: true });
