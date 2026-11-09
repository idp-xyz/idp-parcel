// 商业版本列表的查询条件、页内排列与行详情的版本池（票 party-commercial-uiux/08；条件下推到 ADR-0144 的目录读口）。
//
// 版本壳各册与策略目录按对象号翻页：读口按对象号排，同一对象的各版才不被页界拆开（后端版本壳声明的注释）。组与组的先后照
// 读口答的次序，页面不按别的口径再排一遍；组内新版在上——先比生效起点（晚的在上），起点相同再按版本号自然序（v10 在 v9
// 之上）。读口一次只按一维排，组内的次序它给不了，在这一页之内排。
//
// 「在用」只看服务端答的状态是不是已生效，不在前端按时刻另算：状态由服务端按读取时点导出，页面再算一遍就是第二套口径。
// 「在用 / 其余」下推成读口的 status 维。这一页带着它（或检索词）、或一个对象跨了页界时，缺同一对象的别的版本，而行详情的
// 历史与「收尾…」要的是全部版本，由 versionPoolOf 把不带条件的那一页并进来。
// 纯函数，node:test 直接跑。

import type { CatalogueConditions } from '../catalogue-query';

/** 一行里与版本排列有关的几项；行形各册不同，由调用方投出来。 */
export interface VersionFacts {
  objectId: string;
  version: string;
  /** 服务端的状态码（`EFFECTIVE` 等）；没有状态的册不给。 */
  status?: string;
  effectiveStartsAt?: string;
}

export type VersionStatusFilter = 'all' | 'in-use' | 'other';

// 版本壳 status 维的词表去掉已生效：草稿不进目录、读口筛它答 400，所以「其余」不含草稿。
const statusesOf: Record<VersionStatusFilter, readonly string[]> = {
  all: [],
  'in-use': ['EFFECTIVE'],
  other: ['PUBLISHED', 'EXPIRED', 'RETIRED', 'SUPERSEDED'],
};

/** 「全部 / 在用 / 其余」→ 读口的筛选维；「全部」是这一维缺席。 */
export function versionStatusFilters(filter: VersionStatusFilter): Record<string, readonly string[]> {
  return filter === 'all' ? {} : { status: statusesOf[filter] };
}

/** 版本壳各册一问的条件：检索词、「在用 / 其余」，按对象号排（声明册按所挂靠的对象号，维名由调用方给）。 */
export function versionListConditions(keyword: string, filter: VersionStatusFilter, groupSort = 'objectId'): CatalogueConditions {
  return { q: keyword, sort: groupSort, filters: versionStatusFilters(filter) };
}

const versionOrder = new Intl.Collator('en', { numeric: true, sensitivity: 'base' });

/** 按对象排在一起、组内新版在上；组的先后照输入里各对象第一次出现的次序。不改原数组。 */
export function groupedByObject<Row>(rows: readonly Row[], factsOf: (row: Row) => VersionFacts): Row[] {
  const firstSeen = new Map<string, number>();
  rows.forEach((row, index) => {
    const objectId = factsOf(row).objectId;
    if (!firstSeen.has(objectId)) firstSeen.set(objectId, index);
  });
  return [...rows].sort((left, right) => {
    const a = factsOf(left);
    const b = factsOf(right);
    const byObject = (firstSeen.get(a.objectId) ?? 0) - (firstSeen.get(b.objectId) ?? 0);
    if (byObject !== 0) return byObject;
    const byStart = (b.effectiveStartsAt ?? '').localeCompare(a.effectiveStartsAt ?? '');
    if (byStart !== 0) return byStart;
    return versionOrder.compare(b.version, a.version);
  });
}

/**
 * 行详情的版本池：这一页的行在前，不带条件的那一页（取数层缓存的目录第一页）里这一页没有的行跟在后面，同一版只留一份。
 * 两边都只是一页：一个对象的版本多到落在两页之外时，历史照旧缺那几版。
 */
export function versionPoolOf<Row>(rows: readonly Row[], unfiltered: readonly Row[], keyOf: (row: Row) => string): Row[] {
  const seen = new Set(rows.map(keyOf));
  return [...rows, ...unfiltered.filter((row) => !seen.has(keyOf(row)))];
}
