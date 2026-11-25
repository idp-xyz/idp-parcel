// 退货管理这一页里两种单据的地址（纯 TypeScript，node:test 直接测）。
//
// 退货预报与退货到仓同在一个模块下，所以单据键第一段是种类词、第二段才是标识：`退货预报/<预报号>`、`退货到仓/<作业事实号>`。
// 键会原样显示在标签上，种类词就用中文。预报号取列表上看到的那一个（有预报号就是预报号，没有就是预报标识），读法在
// return-flow.ts 的 findReturnAdvice。种类词只在这里定义一处，列表的链接、模块页的分派与别的页指过来都读它。

export const returnsModule = 'warehouse-returns';

export const returnAdviceKind = '退货预报';
export const returnReceiptKind = '退货到仓';

export type ReturnsTarget =
  | { kind: 'advice'; id: string }
  | { kind: 'receipt'; id: string }
  /** 第一段不是两种种类词里的任何一个：地址写错了，不猜它想开哪一张。 */
  | { kind: 'unknown'; word: string };

/**
 * 单据键的各段认成要开的是哪一种、哪一张。标识缺段时 id 是空串，由单据页说「地址里缺……」，而不是退回列表：
 * 退回会让人以为这张单据本来就没有地址。
 */
export function returnsTarget(parts: readonly string[]): ReturnsTarget {
  const word = parts[0] ?? '';
  const id = parts[1] ?? '';
  switch (word) {
    case returnAdviceKind:
      return { kind: 'advice', id };
    case returnReceiptKind:
      return { kind: 'receipt', id };
    default:
      return { kind: 'unknown', word };
  }
}
