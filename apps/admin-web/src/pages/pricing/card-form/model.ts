// 价卡编辑器（「新建价卡」签）的表单状态。
//
// 状态按运营的理解分块（基本信息、基础运费、重量规则、费用项、总额上下限、参考数据），不照模板的表分：表与列、标识的
// 互相引用只在「表单 → 工作簿」那一步（workbook.ts）生成，页面上没有任何标识要运营去配。
//
// 全部取值都是用户在页面上输入的文本，租户取值不带默认（ADR-0146）：只有「机制」层的初值——计价对象不预选、基础运费代码
// 用产品固定的 BASE_FREIGHT、币种与单位留空等用户选。

export type StepId = 'basics' | 'base' | 'weight' | 'fees' | 'bounds' | 'refs' | 'preview';

export const stepOrder: readonly StepId[] = ['basics', 'base', 'weight', 'fees', 'bounds', 'refs', 'preview'];

export const stepTitles: Readonly<Record<StepId, string>> = {
  basics: '基本信息',
  base: '基础运费',
  weight: '重量规则',
  fees: '其他费用项',
  bounds: '总额上下限',
  refs: '参考数据',
  preview: '预览与存草稿',
};

let uidCounter = 0;
/** 行与费用项的稳定键：只在页面会话内唯一，不进工作簿。 */
export function nextUid(prefix: string): string {
  uidCounter += 1;
  return `${prefix}${uidCounter}`;
}

export interface BasicsState {
  planId: string;
  planVersion: string;
  scope: string;
  direction: string;
  purpose: string;
  aggregation: string;
  baseChargeCode: string;
  currency: string;
  weightUnit: string;
  /** 只填日期按当天零点（UTC），要写明时区就填完整时刻。 */
  periodStartsAt: string;
  periodEndsAt: string;
  authId: string;
  authVersion: string;
}

/** 一行价表：单元格按模板列键存，列由规格口给。 */
export interface RateRow {
  uid: string;
  cells: Record<string, string>;
}

export interface BaseRateState {
  family: string;
  rows: RateRow[];
}

export interface RoundingSegment {
  uid: string;
  mode: string;
  increment: string;
  /** 本段上界；空即最后一段、不封顶。 */
  maximum: string;
}

export interface WeightState {
  method: string;
  divisor: string;
  lengthUnit: string;
  chargeable: RoundingSegment[];
  volumetric: RoundingSegment[];
}

export type FeeKind = 'FIXED' | 'CONDITIONAL' | 'PERCENT' | 'TABLE_ZONE' | 'TABLE_REMOTE';

export interface ConditionRow {
  uid: string;
  source: string;
  operator: string;
  /** 数值特征的阈值，或类别特征的取值。 */
  value: string;
  unit: string;
}

export interface FeeItemState {
  uid: string;
  kind: FeeKind;
  name: string;
  code: string;
  /** 费用代码被人手改过：之后改名称不再覆盖它。 */
  codeEdited: boolean;
  /** 固定金额与条件附加费的金额。 */
  amount: string;
  /** 按基础运费的百分比：费率取序列还是固定百分比。 */
  percentMode: 'SERIES' | 'FIXED';
  seriesId: string;
  seriesFactor: string;
  percentage: string;
  /** 百分比的基数：只算基础运费，还是其他已计收费用之和。 */
  basis: 'BASE' | 'ALL';
  /** 按分区 / 按偏远档位查价表的行：cells 取 zone、minimum、maximum、amount。 */
  tableRows: RateRow[];
  combine: 'ALL_OF' | 'ANY_OF';
  conditions: ConditionRow[];
  /** 互斥组名；空即不与其他费用项互斥。 */
  groupName: string;
  priority: string;
}

export interface BoundsState {
  minimum: string;
  maximum: string;
}

export interface AmountRoundingState {
  enabled: boolean;
  mode: string;
  increment: string;
  points: string[];
}

export interface RefsState {
  zoneCatalogue: string;
  remoteTierCatalogue: string;
}

export interface CardFormState {
  basics: BasicsState;
  base: BaseRateState;
  weight: WeightState;
  fees: FeeItemState[];
  bounds: BoundsState;
  amountRounding: AmountRoundingState;
  refs: RefsState;
  /**
   * 标识提示：槽名 → 标识。表单生成的标识按方案标识与序号推出来，推不出来的才查这里。只给「由现成工作簿还原」用
   * （验收测试与将来的改版），页面上的表单永远是空的。
   */
  idHints: Record<string, string>;
}

/** 基础运费代码是产品固定的计价代码（基础运费行的名字），页面上不让运营改。 */
export const defaultBaseChargeCode = 'BASE_FREIGHT';

export function newRateRow(cells: Record<string, string> = {}): RateRow {
  return { uid: nextUid('r'), cells };
}

export function newSegment(partial: Partial<Omit<RoundingSegment, 'uid'>> = {}): RoundingSegment {
  return { uid: nextUid('s'), mode: '', increment: '', maximum: '', ...partial };
}

export function newCondition(partial: Partial<Omit<ConditionRow, 'uid'>> = {}): ConditionRow {
  return { uid: nextUid('c'), source: '', operator: '', value: '', unit: '', ...partial };
}

export function emptyState(): CardFormState {
  return {
    basics: {
      planId: '',
      planVersion: '',
      scope: '',
      direction: '',
      purpose: '',
      aggregation: '',
      baseChargeCode: defaultBaseChargeCode,
      currency: '',
      weightUnit: '',
      periodStartsAt: '',
      periodEndsAt: '',
      authId: '',
      authVersion: '',
    },
    base: { family: '', rows: [] },
    weight: { method: '', divisor: '', lengthUnit: '', chargeable: [newSegment()], volumetric: [newSegment()] },
    fees: [],
    bounds: { minimum: '', maximum: '' },
    amountRounding: { enabled: false, mode: '', increment: '', points: [] },
    refs: { zoneCatalogue: '', remoteTierCatalogue: '' },
    idHints: {},
  };
}

export const feeKindLabels: Readonly<Record<FeeKind, string>> = {
  FIXED: '固定金额',
  CONDITIONAL: '条件附加费',
  PERCENT: '按基础运费的百分比',
  TABLE_ZONE: '按分区和重量查价表',
  TABLE_REMOTE: '按偏远档位查价表',
};

export const feeKindHints: Readonly<Record<FeeKind, string>> = {
  FIXED: '每件固定收一笔，不看条件',
  CONDITIONAL: '满足条件时收一笔固定金额，如超尺寸、超重',
  PERCENT: '按基础运费乘一个比例，如燃油附加费',
  TABLE_ZONE: '满足条件时，按分区和重量段在价表里查金额',
  TABLE_REMOTE: '按收件地址的偏远档位查金额，如一档 12、二档 28',
};

export const feeKindOrder: readonly FeeKind[] = ['FIXED', 'CONDITIONAL', 'PERCENT', 'TABLE_ZONE', 'TABLE_REMOTE'];

/** 条件附加费要先看到一行可改的条件；其余需要条件的类型起手给「每件都收」那一行，由运营改。 */
export function alwaysCondition(weightUnit: string): ConditionRow {
  return newCondition({ source: 'ACTUAL_WEIGHT', operator: 'GT', value: '0', unit: weightUnit });
}

export function newFee(kind: FeeKind, weightUnit: string): FeeItemState {
  const needsCondition = kind === 'CONDITIONAL' || kind === 'PERCENT' || kind === 'TABLE_ZONE';
  return {
    uid: nextUid('f'),
    kind,
    name: '',
    code: '',
    codeEdited: false,
    amount: '',
    percentMode: 'SERIES',
    seriesId: '',
    seriesFactor: '1',
    percentage: '',
    basis: 'BASE',
    tableRows: kind === 'TABLE_ZONE' || kind === 'TABLE_REMOTE' ? [newRateRow({ minimum: '0' })] : [],
    combine: 'ALL_OF',
    conditions: kind === 'CONDITIONAL' ? [newCondition()] : needsCondition ? [alwaysCondition(weightUnit)] : [],
    groupName: '',
    priority: '',
  };
}

/** 费用项是否带触发条件（固定金额不带；偏远档位的条件由价表里的档位推出）。 */
export function feeHasConditionBuilder(kind: FeeKind): boolean {
  return kind === 'CONDITIONAL' || kind === 'PERCENT' || kind === 'TABLE_ZONE';
}

export function feeHasGroup(kind: FeeKind): boolean {
  return kind !== 'FIXED';
}

// ---- 费用代码建议 ----

const codeKeywords: readonly (readonly [string, string])[] = [
  ['燃油', 'FUEL_SURCHARGE'],
  ['偏远', 'REMOTE_AREA_SURCHARGE'],
  ['超尺寸', 'OVERSIZE_SURCHARGE'],
  ['超长', 'OVERSIZE_SURCHARGE'],
  ['超重', 'OVERWEIGHT_SURCHARGE'],
  ['住宅', 'RESIDENTIAL_DELIVERY_SURCHARGE'],
];

/**
 * 按名称建议一个费用代码（大写字母、数字、下划线）。认得的名称用产品里已有的代码；纯英文名转大写下划线；其余按类型给一个
 * 通用代码。与已有代码重名时补序号。只是建议，运营可以改。
 */
export function suggestChargeCode(kind: FeeKind, name: string, taken: readonly string[]): string {
  const text = name.trim();
  let base = '';
  for (const [keyword, code] of codeKeywords) {
    if (text.includes(keyword)) {
      base = code;
      break;
    }
  }
  if (base === '' && /^[A-Za-z][A-Za-z0-9 _-]*$/.test(text)) {
    base = text.toUpperCase().replace(/[ -]+/g, '_');
  }
  if (base === '') base = kind === 'FIXED' ? 'FIXED_CHARGE' : 'OTHER_SURCHARGE';
  if (!taken.includes(base)) return base;
  for (let n = 2; ; n += 1) {
    if (!taken.includes(`${base}_${n}`)) return `${base}_${n}`;
  }
}

export const chargeCodePattern = /^[A-Z][A-Z0-9_]*$/;
