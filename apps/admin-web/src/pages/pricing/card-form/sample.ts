// 「从示例开始」载入的演示示例：一张带燃油、超尺寸、超重、偏远二档和最低收费的价卡，帮运营看懂每一步怎么填。
// 示例里的方案标识、授权、参考数据标识和全部数值都是合成演示值（SYN- 前缀，证据层级 S），不是任何租户的真实价目，
// 也不是随产品发布的默认价目——载入后每一格都可以改。

import {
  alwaysCondition,
  emptyState,
  newCondition,
  newRateRow,
  newSegment,
  nextUid,
  type CardFormState,
  type FeeItemState,
  type FeeKind,
} from './model';

export const sampleNote = '演示示例，合成值：方案标识、授权和各项金额都只是演示，不是真实价目。';

function fee(kind: FeeKind, partial: Partial<FeeItemState>): FeeItemState {
  return {
    uid: nextUid('f'),
    kind,
    name: '',
    code: '',
    codeEdited: true,
    amount: '',
    percentMode: 'SERIES',
    seriesId: '',
    seriesFactor: '1',
    percentage: '',
    basis: 'BASE',
    tableRows: [],
    combine: 'ALL_OF',
    conditions: [],
    groupName: '',
    priority: '',
    ...partial,
  };
}

export function sampleState(): CardFormState {
  const state = emptyState();
  Object.assign(state.basics, {
    planId: 'SYN-PLAN-CN-SG-FORM-DEMO-01',
    planVersion: 'v1',
    scope: 'SYN-SCOPE-04',
    direction: 'SELL',
    purpose: 'CUSTOMER_CHARGE',
    aggregation: 'PER_PACKAGE',
    currency: 'CNY',
    weightUnit: 'KG',
    periodStartsAt: '2026-01-01',
    periodEndsAt: '',
    authId: 'SYN-AUTH-PRICE-DIR-01',
    authVersion: 'v1',
  });
  state.base = {
    family: 'FIRST_CONTINUE',
    rows: [
      newRateRow({ zone: 'Z1', firstWeight: '0.5', firstAmount: '55', step: '0.5', stepAmount: '18' }),
      newRateRow({ zone: 'Z2', firstWeight: '0.5', firstAmount: '48', step: '0.5', stepAmount: '15' }),
    ],
  };
  state.weight = {
    method: 'MAX',
    divisor: '5000',
    lengthUnit: 'CM',
    chargeable: [newSegment({ mode: 'CEILING', increment: '0.5' })],
    volumetric: [newSegment({ mode: 'CEILING', increment: '0.1' })],
  };
  const oversizeGroup = 'SYN-GROUP-OVERSIZE-OVERWEIGHT';
  state.fees = [
    fee('PERCENT', {
      name: '燃油附加费（合成演示值）',
      code: 'FUEL_SURCHARGE',
      percentMode: 'SERIES',
      seriesId: 'SYN-SERIES-FUEL-FULL-01',
      seriesFactor: '1',
      basis: 'BASE',
      conditions: [alwaysCondition('KG')],
    }),
    fee('CONDITIONAL', {
      name: '超尺寸费（合成演示值）',
      code: 'OVERSIZE_SURCHARGE',
      amount: '35',
      combine: 'ANY_OF',
      conditions: [
        newCondition({ source: 'LONGEST_SIDE', operator: 'GT', value: '100', unit: 'CM' }),
        newCondition({ source: 'LENGTH_AND_GIRTH', operator: 'GT', value: '300', unit: 'CM' }),
      ],
      groupName: oversizeGroup,
      priority: '1',
    }),
    fee('CONDITIONAL', {
      name: '超重费（合成演示值）',
      code: 'OVERWEIGHT_SURCHARGE',
      amount: '25',
      conditions: [newCondition({ source: 'ACTUAL_WEIGHT', operator: 'GT', value: '20', unit: 'KG' })],
      groupName: oversizeGroup,
      priority: '1',
    }),
    fee('TABLE_REMOTE', {
      name: '偏远地区费（合成演示值）',
      code: 'REMOTE_AREA_SURCHARGE',
      tableRows: [newRateRow({ zone: 'T1', minimum: '0', amount: '12' }), newRateRow({ zone: 'T2', minimum: '0', amount: '28' })],
    }),
  ];
  state.bounds = { minimum: '120.99', maximum: '' };
  state.amountRounding = { enabled: true, mode: 'HALF_UP', increment: '0.01', points: ['PER_LINE', 'TOTAL'] };
  state.refs = { zoneCatalogue: 'SYN-CAT-ZONE-CN-SG-FULL', remoteTierCatalogue: 'SYN-CAT-REMOTE-TIER-CN-SG' };
  return state;
}
