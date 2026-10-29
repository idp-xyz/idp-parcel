// 各步的本地校验：只拦显而易见的——必填、数字格式、选择是否在规格口给的可取值里、单位与币种的前后一致。
// 业务上行不行（价表是否重叠、授权是否在册、金额取整是否整倍数……）一律交服务端判，这里不重写（ADR-0101 决定四：
// 预览与录入共用服务端那一份读法）。

import type { PriceCardTemplateSpec } from '../api';
import { featureMeasure } from '../price-card-content';
import { normalizeMoment } from '../series-form';
import { field } from './fields';
import { feePath } from './workbook';
import { chargeCodePattern, feeHasConditionBuilder, feeHasGroup, stepOrder, type CardFormState, type FeeItemState, type StepId } from './model';
import { allowedValues, isTextColumn, rateColumns, sheetNames } from './spec';

export interface Issue {
  field: string;
  /** 位置的人话，不含问题本身。 */
  where: string;
  text: string;
}

const decimal = /^\d+(\.\d+)?$/;
const integer = /^\d+$/;

export const isDecimal = (value: string) => decimal.test(value.trim());

function momentOk(raw: string): boolean {
  const text = normalizeMoment(raw);
  return text !== '' && !Number.isNaN(Date.parse(text));
}

export function validateStep(step: StepId, state: CardFormState, spec: PriceCardTemplateSpec): Issue[] {
  switch (step) {
    case 'basics':
      return basicsIssues(state, spec);
    case 'base':
      return baseIssues(state, spec);
    case 'weight':
      return weightIssues(state, spec);
    case 'fees':
      return state.fees.flatMap((item, index) => feeIssues(state, spec, item, index));
    case 'bounds':
      return boundsIssues(state, spec);
    case 'refs':
      return refsIssues(state);
    case 'preview':
      return [];
  }
}

export function validateAll(state: CardFormState, spec: PriceCardTemplateSpec): Issue[] {
  return stepOrder.flatMap((step) => validateStep(step, state, spec));
}

function pick(issues: Issue[], where: string) {
  return {
    required: (fieldId: string, label: string, value: string) => {
      if (value.trim() === '') issues.push({ field: fieldId, where, text: `请填写${label}` });
    },
    choice: (fieldId: string, label: string, value: string, options: readonly string[]) => {
      if (value.trim() === '') issues.push({ field: fieldId, where, text: `请选择${label}` });
      else if (options.length > 0 && !options.includes(value)) issues.push({ field: fieldId, where, text: `${label}不在可选范围内，请重新选择` });
    },
    number: (fieldId: string, label: string, value: string, required = true) => {
      const text = value.trim();
      if (text === '') {
        if (required) issues.push({ field: fieldId, where, text: `请填写${label}` });
      } else if (!decimal.test(text)) {
        issues.push({ field: fieldId, where, text: `${label}要填数字，如 12 或 0.5（不用逗号、不写单位）` });
      }
    },
  };
}

function basicsIssues(state: CardFormState, spec: PriceCardTemplateSpec): Issue[] {
  const issues: Issue[] = [];
  const { basics } = state;
  const check = pick(issues, '基本信息');
  check.required(field.basics('planId'), '方案标识', basics.planId);
  check.required(field.basics('planVersion'), '版本', basics.planVersion);
  check.required(field.basics('scope'), '适用范围', basics.scope);
  check.choice(field.basics('direction'), '价格方向', basics.direction, allowedValues(spec, sheetNames.card, 'direction'));
  check.choice(field.basics('purpose'), '用途', basics.purpose, allowedValues(spec, sheetNames.card, 'purpose'));
  check.choice(field.basics('aggregation'), '计价对象', basics.aggregation, allowedValues(spec, sheetNames.card, 'aggregation'));
  if (!chargeCodePattern.test(basics.baseChargeCode.trim())) {
    issues.push({ field: field.basics('baseChargeCode'), where: '基本信息', text: '基础运费代码只能是大写字母、数字和下划线，且以字母开头' });
  }
  if (basics.currency.trim() === '') issues.push({ field: field.basics('currency'), where: '基本信息', text: '请选择币种' });
  check.choice(field.basics('weightUnit'), '重量单位', basics.weightUnit, allowedValues(spec, sheetNames.tables, 'weightUnit'));
  if (basics.periodStartsAt.trim() === '') {
    issues.push({ field: field.basics('periodStartsAt'), where: '基本信息', text: '请填写生效起点' });
  } else if (!momentOk(basics.periodStartsAt)) {
    issues.push({ field: field.basics('periodStartsAt'), where: '基本信息', text: '生效起点不是可识别的日期，如 2026-01-01' });
  }
  if (basics.periodEndsAt.trim() !== '') {
    if (!momentOk(basics.periodEndsAt)) {
      issues.push({ field: field.basics('periodEndsAt'), where: '基本信息', text: '生效止点不是可识别的日期；不设止点就留空' });
    } else if (momentOk(basics.periodStartsAt) && Date.parse(normalizeMoment(basics.periodEndsAt)) <= Date.parse(normalizeMoment(basics.periodStartsAt))) {
      issues.push({ field: field.basics('periodEndsAt'), where: '基本信息', text: '生效止点要晚于生效起点' });
    }
  }
  check.required(field.basics('authId'), '价格方向授权标识', basics.authId);
  check.required(field.basics('authVersion'), '价格方向授权版本', basics.authVersion);
  return issues;
}

function baseIssues(state: CardFormState, spec: PriceCardTemplateSpec): Issue[] {
  const issues: Issue[] = [];
  const check = pick(issues, '基础运费');
  check.choice(field.base('family'), '价表类型', state.base.family, allowedValues(spec, sheetNames.tables, 'family'));
  if (state.base.family === '') return issues;
  if (state.base.rows.length === 0) {
    issues.push({ field: field.base('rows'), where: '基础运费', text: '至少要有一行价表' });
    return issues;
  }
  const columns = rateColumns(spec, state.base.family);
  state.base.rows.forEach((row, index) => {
    const rowCheck = pick(issues, `基础运费 · 第 ${index + 1} 行`);
    for (const column of columns) {
      const id = field.baseRow(row.uid, column.key);
      const value = row.cells[column.key] ?? '';
      if (isTextColumn(column.key)) {
        if (column.requirement === 'always') rowCheck.required(id, column.title, value);
      } else {
        rowCheck.number(id, column.title, value, column.requirement === 'always');
      }
    }
  });
  return issues;
}

function weightIssues(state: CardFormState, spec: PriceCardTemplateSpec): Issue[] {
  const issues: Issue[] = [];
  const { weight } = state;
  const check = pick(issues, '重量规则');
  check.choice(field.weight('method'), '计重方法', weight.method, allowedValues(spec, sheetNames.card, 'weightMethod'));
  const modes = allowedValues(spec, sheetNames.weightRounding, 'mode');
  const segmentIssues = (policy: 'chargeable' | 'volumetric', label: string) => {
    const segments = weight[policy];
    if (segments.length === 0) issues.push({ field: field.segment(policy, 'x', 'mode'), where: label, text: '至少要有一段进位' });
    segments.forEach((segment, index) => {
      const where = `${label} · 第 ${index + 1} 段`;
      const segCheck = pick(issues, where);
      segCheck.choice(field.segment(policy, segment.uid, 'mode'), '进位方式', segment.mode, modes);
      segCheck.number(field.segment(policy, segment.uid, 'increment'), '进位单位', segment.increment);
      const last = index === segments.length - 1;
      segCheck.number(field.segment(policy, segment.uid, 'maximum'), '本段上界', segment.maximum, !last);
    });
  };
  segmentIssues('chargeable', '计价重量进位');
  if (weight.method === 'MAX') {
    check.number(field.weight('divisor'), '体积重除数', weight.divisor);
    if (isDecimal(weight.divisor) && Number(weight.divisor) <= 0) {
      issues.push({ field: field.weight('divisor'), where: '重量规则', text: '体积重除数要大于 0' });
    }
    check.choice(field.weight('lengthUnit'), '长度单位', weight.lengthUnit, allowedValues(spec, sheetNames.card, 'volumetricLengthUnit'));
    segmentIssues('volumetric', '体积重进位');
  }
  return issues;
}

function feeIssues(state: CardFormState, spec: PriceCardTemplateSpec, fee: FeeItemState, index: number): Issue[] {
  const issues: Issue[] = [];
  const where = feePath(index, fee);
  const check = pick(issues, where);
  check.required(field.fee(fee.uid, 'name'), '费用名称', fee.name);
  if (!chargeCodePattern.test(fee.code.trim())) {
    issues.push({ field: field.fee(fee.uid, 'code'), where, text: '费用代码只能是大写字母、数字和下划线，且以字母开头' });
  }
  if (fee.kind === 'FIXED' || fee.kind === 'CONDITIONAL') check.number(field.fee(fee.uid, 'amount'), '金额', fee.amount);
  if (fee.kind === 'PERCENT') {
    if (fee.percentMode === 'SERIES') {
      check.required(field.fee(fee.uid, 'seriesId'), '燃油费率序列', fee.seriesId);
      check.number(field.fee(fee.uid, 'seriesFactor'), '系数', fee.seriesFactor);
    } else {
      check.number(field.fee(fee.uid, 'percentage'), '百分比', fee.percentage);
    }
  }
  if (fee.kind === 'TABLE_ZONE' || fee.kind === 'TABLE_REMOTE') {
    const keyLabel = fee.kind === 'TABLE_REMOTE' ? '档位' : '分区';
    if (fee.tableRows.length === 0) issues.push({ field: field.fee(fee.uid, 'rows'), where, text: '价表至少要有一行' });
    fee.tableRows.forEach((row, rowIndex) => {
      const rowCheck = pick(issues, `${where} · 价表第 ${rowIndex + 1} 行`);
      rowCheck.required(field.feeRow(fee.uid, row.uid, 'zone'), keyLabel, row.cells.zone ?? '');
      rowCheck.number(field.feeRow(fee.uid, row.uid, 'minimum'), '重量从', row.cells.minimum ?? '');
      rowCheck.number(field.feeRow(fee.uid, row.uid, 'maximum'), '重量到', row.cells.maximum ?? '', false);
      rowCheck.number(field.feeRow(fee.uid, row.uid, 'amount'), '金额', row.cells.amount ?? '');
    });
  }
  if (feeHasConditionBuilder(fee.kind)) {
    if (fee.conditions.length === 0) issues.push({ field: field.fee(fee.uid, 'conditions'), where, text: '至少要有一行触发条件' });
    fee.conditions.forEach((row, rowIndex) => {
      const rowWhere = `${where} · 条件 ${rowIndex + 1}`;
      const rowCheck = pick(issues, rowWhere);
      rowCheck.choice(field.condition(fee.uid, row.uid, 'source'), '特征', row.source, allowedValues(spec, sheetNames.conditions, 'source'));
      if (row.source === '') return;
      const measure = featureMeasure(row.source);
      if (measure === 'category') {
        rowCheck.required(field.condition(fee.uid, row.uid, 'value'), '取值', row.value);
        return;
      }
      rowCheck.choice(field.condition(fee.uid, row.uid, 'operator'), '比较', row.operator, allowedValues(spec, sheetNames.conditions, 'operator'));
      rowCheck.number(field.condition(fee.uid, row.uid, 'value'), '比较值', row.value);
      rowCheck.choice(field.condition(fee.uid, row.uid, 'unit'), '单位', row.unit, []);
      // 体积重与计价重量是按价表重量单位算出来的，阈值单位必须一致，否则服务端必拒。
      if ((row.source === 'VOLUMETRIC_WEIGHT' || row.source === 'CHARGEABLE_WEIGHT') && row.unit !== '' && row.unit !== state.basics.weightUnit) {
        issues.push({ field: field.condition(fee.uid, row.uid, 'unit'), where: rowWhere, text: '体积重和计价重量的单位要与价表的重量单位一致' });
      }
    });
  }
  if (feeHasGroup(fee.kind) && fee.groupName.trim() !== '' && (!integer.test(fee.priority.trim()) || Number(fee.priority) < 1)) {
    issues.push({ field: field.fee(fee.uid, 'priority'), where, text: '互斥组内要填优先级，用大于 0 的整数（1 最高）' });
  }
  return issues;
}

function boundsIssues(state: CardFormState, spec: PriceCardTemplateSpec): Issue[] {
  const issues: Issue[] = [];
  const check = pick(issues, '总额上下限');
  check.number(field.bounds('minimum'), '最低收费', state.bounds.minimum, false);
  check.number(field.bounds('maximum'), '最高收费', state.bounds.maximum, false);
  if (
    isDecimal(state.bounds.minimum) &&
    isDecimal(state.bounds.maximum) &&
    Number(state.bounds.minimum) > Number(state.bounds.maximum)
  ) {
    issues.push({ field: field.bounds('minimum'), where: '总额上下限', text: '最低收费不能高于最高收费' });
  }
  const rounding = state.amountRounding;
  if (rounding.enabled) {
    const where = '金额取整';
    const roundingCheck = pick(issues, where);
    roundingCheck.choice(field.rounding('mode'), '取整方式', rounding.mode, allowedValues(spec, sheetNames.card, 'amountRoundingMode'));
    roundingCheck.number(field.rounding('increment'), '进位单位', rounding.increment);
    if (rounding.points.length === 0) issues.push({ field: field.rounding('points'), where, text: '至少选一个取整的环节' });
  }
  return issues;
}

function refsIssues(state: CardFormState): Issue[] {
  const issues: Issue[] = [];
  if (state.fees.some((fee) => fee.kind === 'TABLE_REMOTE') && state.refs.remoteTierCatalogue.trim() === '') {
    issues.push({ field: field.refs('remoteTierCatalogue'), where: '参考数据', text: '有「按偏远档位查价表」的费用项，请填写偏远档位目录' });
  }
  return issues;
}
