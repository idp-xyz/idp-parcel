// 盘点与调整这一页里三种单据的地址（纯 TypeScript，node:test 直接测）。
//
// 盘点、库存调整、作业差异同在一个模块下，所以单据键第一段是种类词、第二段才是标识：`盘点/<盘点号>`、`调整/<调整号>`、
// `作业差异/<差异号>`。键会原样显示在标签上，种类词就用中文。种类词只在这里定义一处，列表的链接、模块页的分派与以后别的页
// 指过来都读它。

export const countsAdjustmentsModule = 'warehouse-counts-adjustments';

export const countKind = '盘点';
export const adjustmentKind = '调整';
export const discrepancyKind = '作业差异';

export type CountsAdjustmentsTarget =
  | { kind: 'count'; id: string }
  | { kind: 'adjustment'; id: string }
  | { kind: 'discrepancy'; id: string }
  /** 第一段不是三种种类词里的任何一个：地址写错了，不猜它想开哪一张。 */
  | { kind: 'unknown'; word: string };

/**
 * 单据键的各段认成要开的是哪一种、哪一张。标识缺段时 id 是空串，由单据页说「地址里缺……」，而不是退回列表：
 * 退回会让人以为这张单据本来就没有地址。
 */
export function countsAdjustmentsTarget(parts: readonly string[]): CountsAdjustmentsTarget {
  const word = parts[0] ?? '';
  const id = parts[1] ?? '';
  switch (word) {
    case countKind:
      return { kind: 'count', id };
    case adjustmentKind:
      return { kind: 'adjustment', id };
    case discrepancyKind:
      return { kind: 'discrepancy', id };
    default:
      return { kind: 'unknown', word };
  }
}
