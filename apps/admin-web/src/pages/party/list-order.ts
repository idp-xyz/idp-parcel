// 参与方模块各册页列表的排序比较器与下拉选项形状（票 admin-web-group-legal-entities/09）。纯函数，
// 供 business-party-list.ts 与 party-relationship-list.ts 共用；legal-entity-list.ts（票 01）里同形的
// 私有副本先留着不动——那是另一张票的地盘，切到这里是收口时顺手的一笔，不在本票范围内。

export interface SelectOption<Value extends string> {
  value: Value;
  label: string;
}

/** 字典序，稳定且不依赖 locale——标识是机器串，不按语言排。 */
export const byString = (left: string, right: string) => (left < right ? -1 : left > right ? 1 : 0);

/**
 * 时刻按解析后的毫秒比，不按字符串比：端点经 catalogue_intake.go 的 rfc3339() 用 RFC3339Nano 格式化，
 * 尾零被剪、小数位数不定（`…55Z` / `…55.9Z` / `…55.939Z` 并存），字典序会把 `55Z` 排到 `55.939Z` 之后——
 * 受控 CLI 批量灌入的行落在同一秒，正是默认排序要排的那批（票 01 评审 Standards 1）。
 * 解析不了的值退回字符串比，稳定且可预期。
 */
export const byInstant = (left: string, right: string) => {
  const leftMillis = Date.parse(left);
  const rightMillis = Date.parse(right);
  if (Number.isNaN(leftMillis) || Number.isNaN(rightMillis)) return byString(left, right);
  return leftMillis - rightMillis;
};

/** 包含匹配、不分大小写；空白搜索视为不筛。可选格按空串参与，名称未知的行不抛。 */
export function matchesSearch(search: string, cells: readonly (string | undefined)[]): boolean {
  const needle = search.trim().toLowerCase();
  if (needle === '') return true;
  return cells.some((value) => (value ?? '').toLowerCase().includes(needle));
}
