// 商业版本列表的排列与「在用」筛选（票 party-commercial-uiux/08）。
//
// 同一个对象的各版连续出现、组内新版在上，「这个对象此刻用的是哪一版」才看得出来；读口的顺序是登记顺序，同一对象的
// 各版会夹着别的对象出现。组与组按对象标识排。组内先比生效起点（晚的在上），起点相同再按版本号自然序（v10 在 v9 之上）。
//
// 「在用」只看服务端答的状态是不是已生效，不在前端按时刻另算：状态由服务端按读取时点导出，页面再算一遍就是第二套口径。
// 纯函数，node:test 直接跑。

/** 一行里与版本排列有关的几项；行形各册不同，由调用方投出来。 */
export interface VersionFacts {
  objectId: string;
  version: string;
  /** 服务端的状态码（`EFFECTIVE` 等）；没有状态的册不给，那几册只排不筛。 */
  status?: string;
  effectiveStartsAt?: string;
}

export type VersionStatusFilter = 'all' | 'in-use' | 'other';

const inUseStatus = 'EFFECTIVE';

const versionOrder = new Intl.Collator('en', { numeric: true, sensitivity: 'base' });

/** 按对象排在一起、组内新版在上。不改原数组。 */
export function groupedByObject<Row>(rows: readonly Row[], factsOf: (row: Row) => VersionFacts): Row[] {
  return [...rows].sort((left, right) => {
    const a = factsOf(left);
    const b = factsOf(right);
    const byObject = versionOrder.compare(a.objectId, b.objectId);
    if (byObject !== 0) return byObject;
    const byStart = (b.effectiveStartsAt ?? '').localeCompare(a.effectiveStartsAt ?? '');
    if (byStart !== 0) return byStart;
    return versionOrder.compare(b.version, a.version);
  });
}

export function matchesVersionStatus(facts: VersionFacts, filter: VersionStatusFilter): boolean {
  if (filter === 'all') return true;
  return (facts.status === inUseStatus) === (filter === 'in-use');
}

export interface VersionStatusCounts {
  all: number;
  inUse: number;
  other: number;
}

export function versionStatusCounts<Row>(rows: readonly Row[], factsOf: (row: Row) => VersionFacts): VersionStatusCounts {
  const inUse = rows.filter((row) => factsOf(row).status === inUseStatus).length;
  return { all: rows.length, inUse, other: rows.length - inUse };
}

/** 这一册的行带不带状态：一行都没有状态的册不显「在用」筛选（空册也不显，没东西可筛）。 */
export function hasVersionStatus<Row>(rows: readonly Row[], factsOf: (row: Row) => VersionFacts | undefined): boolean {
  return rows.some((row) => factsOf(row)?.status !== undefined);
}

/** 右端计数：没筛掉东西时只报总数，筛掉了再报当前显示几份——计数跟着筛选走，不让人以为表里还有没显示的行。 */
export function versionCountSummary(total: number, visible: number, measure: string, noun: string): string {
  return visible === total
    ? `共 ${total} ${measure}${noun}`
    : `共 ${total} ${measure}${noun}，当前显示 ${visible} ${measure}`;
}
