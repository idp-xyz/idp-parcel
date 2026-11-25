// 加工与换标这一页里三种单据的地址（纯 TypeScript，node:test 直接测）。
//
// 指令、冻结、库内作业同在一个模块下，所以单据键第一段是种类词、第二段才是标识：`指令/<指令号>`、`冻结/<冻结号>`、
// `库内作业/<作业事实号>`。键会原样显示在标签上，种类词就用中文。种类词只在这里定义一处，列表的链接、模块页的分派与
// 别的页指过来（作业差异页指向它来自的库内作业）都读它。

export const processingModule = 'warehouse-processing';

export const instructionKind = '指令';
export const freezeKind = '冻结';
export const internalJobKind = '库内作业';

export type ProcessingTarget =
  | { kind: 'instruction'; id: string }
  | { kind: 'freeze'; id: string }
  | { kind: 'internalJob'; id: string }
  /** 第一段不是三种种类词里的任何一个：地址写错了，不猜它想开哪一张。 */
  | { kind: 'unknown'; word: string };

/**
 * 单据键的各段认成要开的是哪一种、哪一张。标识缺段时 id 是空串，由单据页说「地址里缺……」，而不是退回列表：
 * 退回会让人以为这张单据本来就没有地址。
 */
export function processingTarget(parts: readonly string[]): ProcessingTarget {
  const word = parts[0] ?? '';
  const id = parts[1] ?? '';
  switch (word) {
    case instructionKind:
      return { kind: 'instruction', id };
    case freezeKind:
      return { kind: 'freeze', id };
    case internalJobKind:
      return { kind: 'internalJob', id };
    default:
      return { kind: 'unknown', word };
  }
}
