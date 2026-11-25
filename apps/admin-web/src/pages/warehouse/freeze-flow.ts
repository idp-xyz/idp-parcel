// 库存冻结：生命周期在进度条上的落点与列表每行、单据页头区共用的下一步（纯 TypeScript，node:test 直接测）。
//
// 一份冻结落册后停在冻结中：冻着的数量不能被出库分配，库内作业也取不走它。解冻是整份解冻，不能只解一部分；原冻结留着，
// 数量回到可用量。已解冻之后不再变，也不会再冻回去——要再冻就是新的一份冻结。

import type { DocumentTrack, TrackStep } from './document-track';

export const freezeSteps: readonly TrackStep[] = [
  { id: 'FROZEN', label: '冻结中' },
  { id: 'UNFROZEN', label: '已解冻' },
];

/**
 * 冻结中卡在等人解冻：数量在这期间不可用，用待办色标出。读口没答出的陌生状态词原样示在第一步上，不替它猜走到了哪。
 */
export function freezeTrack(status: string): DocumentTrack {
  switch (status) {
    case 'FROZEN':
      return { steps: freezeSteps, currentId: 'FROZEN', currentLabel: '冻结中' };
    case 'UNFROZEN':
      return { steps: freezeSteps, currentId: 'UNFROZEN' };
    default:
      return { steps: freezeSteps, currentId: 'FROZEN', currentLabel: status };
  }
}

export interface FreezeNextStep {
  label: string;
}

/** 只有冻结中的有下一步：解冻。已解冻的不会再变。 */
export function freezeNextStep(status: string): FreezeNextStep | null {
  return status === 'FROZEN' ? { label: '解冻' } : null;
}

/** 这份冻结冻了多久，从哪一刻算：从冻结时刻算。已解冻的不在冻着，答 undefined。 */
export function freezeWaitingSince(freeze: { status: string; frozenAt: string }): string | undefined {
  return freeze.status === 'FROZEN' ? freeze.frozenAt : undefined;
}

/** 冻的是哪一种：按商品格冻（库位上的一格货，带数量），还是按整箱冻（箱号，数量恒为一箱，不带库位）。陌生的对象不当整箱。 */
export function freezeIsCarton(freeze: { target: string }): boolean {
  return freeze.target === 'CARTON';
}

/** 冻结数量连单位：整箱冻结恒为一箱，写箱；商品格冻结写数量与单位，没有单位只写数量。 */
export function freezeQuantityLabel(freeze: { target: string; quantity: number; unit?: string }): string {
  if (freezeIsCarton(freeze)) return `${freeze.quantity} 箱`;
  return `${freeze.quantity}${freeze.unit ? ` ${freeze.unit}` : ''}`;
}
