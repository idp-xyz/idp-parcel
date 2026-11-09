// 参与方模块各册页列表的排序比较器、搜索匹配与筛选下拉的形状（票 admin-web-group-legal-entities/09 立，票 13 第 8 条
// 把法人页那份私有副本切过来并统一筛选码的类型）。纯函数，各册的 *-list.ts（legal-entity-list.ts / business-party-list.ts /
// party-relationship-list.ts……）共用。参与方各册的表头排序经 catalogueSortOf 下推到目录读口（ADR-0144）；比较器与搜索
// 匹配给在一页之内排、筛的调用方。

import type { WorkbenchSort } from '../../templates/workbench';

export interface SelectOption<Value extends string> {
  value: Value;
  label: string;
}

/**
 * 筛选下拉的值：「全部」或词表里的一个码。码的封闭集由 presentation.ts 的词表拥有（CONTEXT 原词在前端的唯一一处），
 * `Code` 从它的键派生——任何列表模块不再抄一份联合类型，抄了就会在词表扩格时漏一处。
 */
export type CodeFilter<Code extends string> = 'ALL' | Code;

/**
 * 筛选选项表：「全部」在首位，其后是词表的每个码沿登记顺序各一项（那顺序就是 CONTEXT 里列举的顺序），词就是
 * 词表里的词。页面直接渲染这张表，不另抄。
 */
export function codeFilterOptions<Code extends string>(
  table: Record<Code, string>,
  allLabel: string,
): readonly SelectOption<CodeFilter<Code>>[] {
  // Object.keys 交回 string[]；键的封闭集已由 Record<Code, string> 在类型上钉住，这一步只是把它说回去。
  const codes = Object.keys(table) as Code[];
  return [{ value: 'ALL', label: allLabel }, ...codes.map((value) => ({ value, label: table[value] }))];
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

/**
 * 表头列键 → 目录读口声明的可排维。只收读口点了名的维：别的列在页面上排，排的只是这一页，与读口给的次序打架，所以那些列
 * 不可排。
 */
export type CatalogueSortColumns = Readonly<Record<string, string>>;

// 只认对照自己的键：列键是任意串，按下标取会摸到原型链上的同名成员。
function sortableColumnOf(columns: CatalogueSortColumns, key: string): string | undefined {
  return Object.prototype.hasOwnProperty.call(columns, key) ? columns[key] : undefined;
}

/**
 * 表头排序编成读口的 sort（ADR-0144 决定三：一次一维，前缀 `-` 为倒序）。列不在对照里交 undefined，即读口的缺省序；编出来正是
 * 读口缺省序（`readDefault`，照后端声明抄）时也不写进查询串——不带条件的第一页是裸路径，与不翻页的调用方问的是同一个地址。
 */
export function catalogueSortOf(sort: WorkbenchSort, columns: CatalogueSortColumns, readDefault?: string): string | undefined {
  const field = sortableColumnOf(columns, sort.key);
  if (field === undefined) return undefined;
  const encoded = sort.dir < 0 ? `-${field}` : field;
  return encoded === readDefault ? undefined : encoded;
}

/** 记下的排序（换标签回来恢复的那一份）落在不可排的列上时退回页面缺省序：表头的箭头与实际次序才对得上。 */
export function sortableOr(sort: WorkbenchSort, columns: CatalogueSortColumns, fallback: WorkbenchSort): WorkbenchSort {
  return sortableColumnOf(columns, sort.key) !== undefined ? sort : fallback;
}
