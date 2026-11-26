// 实时库存里库存格、整箱、汇总、占用四种看法的翻页。读口对这四种一次答回整份、没有游标，所以翻页是在已经取回的行上切，
// 不再问服务端；行数少了（刷新、解冻之后）当前页会落到末页之外，取页时收回到末页。流水由读口按游标翻页，不走这里。

/** 一页摆多少行。读口不分页，这个数只管页面上一屏的长度。 */
export const stockPageSize = 50;

export function stockPageCount(total: number, size: number = stockPageSize): number {
  if (size <= 0) return 1;
  return Math.max(1, Math.ceil(total / size));
}

/** 把要看的页收进 1 到末页之间。 */
export function stockPageClamp(page: number, total: number, size: number = stockPageSize): number {
  return Math.min(Math.max(1, Math.trunc(page)), stockPageCount(total, size));
}

export function stockPageSlice<Row>(rows: readonly Row[], page: number, size: number = stockPageSize): Row[] {
  const current = stockPageClamp(page, rows.length, size);
  return rows.slice((current - 1) * size, current * size);
}

/** 翻页条上的一句，写法与游标翻页条一致。取回的就是整份，所以说得出「共」多少条。 */
export function stockPageSummary(page: number, total: number, size: number = stockPageSize): string {
  const current = stockPageClamp(page, total, size);
  return `第 ${current} / ${stockPageCount(total, size)} 页 · 共 ${total} 条`;
}
