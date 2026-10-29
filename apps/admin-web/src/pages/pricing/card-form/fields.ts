// 表单控件的定位串。工作簿问题要标回控件、前端校验要标在控件上、点问题要滚到控件，三处用同一串，所以只在这里定义。

export const field = {
  basics: (key: string) => `basics.${key}`,
  base: (key: string) => `base.${key}`,
  baseRow: (rowUid: string, column: string) => `base.row.${rowUid}.${column}`,
  weight: (key: string) => `weight.${key}`,
  segment: (policy: 'chargeable' | 'volumetric', segmentUid: string, key: string) => `weight.${policy}.${segmentUid}.${key}`,
  fee: (feeUid: string, key: string) => `fee.${feeUid}.${key}`,
  condition: (feeUid: string, rowUid: string, key: string) => `fee.${feeUid}.cond.${rowUid}.${key}`,
  feeRow: (feeUid: string, rowUid: string, column: string) => `fee.${feeUid}.row.${rowUid}.${column}`,
  bounds: (key: string) => `bounds.${key}`,
  rounding: (key: string) => `rounding.${key}`,
  refs: (key: string) => `refs.${key}`,
};

import type { StepId } from './model';

/** 控件属于向导的哪一步；定位串的第一段定。 */
export function stepOfField(fieldId: string): StepId {
  const head = fieldId.split('.')[0];
  switch (head) {
    case 'basics':
      return 'basics';
    case 'base':
      return 'base';
    case 'weight':
      return 'weight';
    case 'bounds':
    case 'rounding':
      return 'bounds';
    case 'refs':
      return 'refs';
    case 'step': {
      const step = fieldId.split('.')[1];
      return (step as StepId | undefined) ?? 'fees';
    }
    default:
      return 'fees';
  }
}
