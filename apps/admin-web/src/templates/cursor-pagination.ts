// 游标翻页的纯逻辑（票 catalogue-read-pagination/04；契约是 ADR-0144 决定一、五、八，这里只引不复述）。服务端只给向后的游标，
// 「上一页」由客户端记住走过的游标自己回退；游标里带着排序与筛选的摘要，换了条件还拿旧游标是坏请求，所以条件一变就回第一页。
// 不引 React，run-tests 的 CommonJS 发射能直接加载；翻页条的渲染在 ListPageTemplate（游标形的 pagination）。

/**
 * 走过的游标。`afters[i]` 是取第 i + 2 页时带的 `after`，第一页不带；`query` 是造这条轨迹时的查询条件签名（排序、筛选、检索词
 * 拼成的串，由调用方定），条件一变整条轨迹作废。
 */
export interface CursorTrail {
  readonly query: string;
  readonly afters: readonly string[];
}

export function startCursorTrail(query: string): CursorTrail {
  return { query, afters: [] };
}

/** 按本次的查询条件取轨迹：条件没变原样返回，变了回第一页。 */
export function cursorTrailFor(trail: CursorTrail, query: string): CursorTrail {
  return trail.query === query ? trail : startCursorTrail(query);
}

/** 取当前页要带的 `after`；第一页为 null。 */
export function currentCursorAfter(trail: CursorTrail): string | null {
  return trail.afters.length === 0 ? null : trail.afters[trail.afters.length - 1];
}

/** 当前第几页（1 起）。 */
export function cursorPageNumber(trail: CursorTrail): number {
  return trail.afters.length + 1;
}

/** 下一页：把本页答复的 `next` 压栈；`next` 为 null（已到末页）原样返回。 */
export function nextCursorPage(trail: CursorTrail, next: string | null): CursorTrail {
  return next === null ? trail : { query: trail.query, afters: [...trail.afters, next] };
}

/** 上一页：出栈；已在第一页原样返回。 */
export function previousCursorPage(trail: CursorTrail): CursorTrail {
  return trail.afters.length === 0 ? trail : { query: trail.query, afters: trail.afters.slice(0, -1) };
}

/** 翻页条上两个钮能不能按：第一页没有上一页，`next` 为 null 即末页、没有下一页。 */
export function cursorPagerControls(page: number, next: string | null): { canPrevious: boolean; canNext: boolean } {
  return { canPrevious: page > 1, canNext: next !== null };
}

/** 总页数，由答复回显的页大小与总数算；总数为 null（读口不给）即 null，不去猜。总数为 0 也算一页。 */
export function cursorTotalPages(size: number, total: number | null): number | null {
  if (total === null || size <= 0) return null;
  return Math.max(1, Math.ceil(total / size));
}

/** 翻页条上的一句：「第 2 / 5 页 · 共 812 条」；总数为 null 时只说「第 2 页」。 */
export function cursorPageSummary(page: number, size: number, total: number | null): string {
  const pages = cursorTotalPages(size, total);
  return pages === null ? `第 ${page} 页` : `第 ${page} / ${pages} 页 · 共 ${total} 条`;
}
