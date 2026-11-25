// 建账导入单：生命周期在进度条上的落点，与单据上的几个合计（纯 TypeScript，node:test 直接测）。
//
// 一张导入提交后停在待批，批准才入账、驳回就此结束，没有第三条路；已入账与已驳回之后不再变。

import type { DocumentTrack, TrackStep } from './document-track';

export const stockImportSteps: readonly TrackStep[] = [
  { id: 'PENDING', label: '待批' },
  { id: 'POSTED', label: '已入账' },
];

/**
 * 待批用待办色标出：它在等人批准或驳回，是这张单上唯一要人动手的一步。驳回是流程停下，用红色的停止标记，
 * 步子都不算走过。读口没答出的陌生状态词原样示在第一步上，不替它猜走到了哪。
 */
export function stockImportTrack(status: string): DocumentTrack {
  switch (status) {
    case 'PENDING':
      return { steps: stockImportSteps, currentId: 'PENDING', currentLabel: '待批' };
    case 'POSTED':
      return { steps: stockImportSteps, currentId: 'POSTED' };
    case 'REJECTED':
      return { steps: stockImportSteps, currentId: 'PENDING', halt: { label: '已驳回', tone: 'danger' } };
    default:
      return { steps: stockImportSteps, currentId: 'PENDING', currentLabel: status };
  }
}

interface ImportSource {
  goodsLines: ReadonlyArray<{ locationCode: string; goodsCode: string }>;
  cartonLines: ReadonlyArray<{ locationCode: string; contents: ReadonlyArray<{ goodsCode: string }> }>;
}

export interface StockImportTotals {
  goodsLines: number;
  cartonLines: number;
  /** 涉及几种商品：按商品行的商品与整箱里装的商品合起来去重。 */
  goods: number;
  /** 涉及几个库位：商品行与整箱行的库位合起来去重。 */
  locations: number;
}

/**
 * 批准的人要先知道这一笔会动多大的账：几行、几种商品、几个库位。数量不在这里合计——商品行的单位各不相同，
 * 件与箱加起来没有意义。
 */
export function stockImportTotals(source: ImportSource): StockImportTotals {
  const goods = new Set<string>();
  const locations = new Set<string>();
  for (const line of source.goodsLines) {
    goods.add(line.goodsCode);
    locations.add(line.locationCode);
  }
  for (const line of source.cartonLines) {
    locations.add(line.locationCode);
    for (const item of line.contents) goods.add(item.goodsCode);
  }
  return {
    goodsLines: source.goodsLines.length,
    cartonLines: source.cartonLines.length,
    goods: goods.size,
    locations: locations.size,
  };
}
