// 条件构造器边填边预览的人话：复用价卡详情的描述函数（price-card-content.ts 的 describeTrigger），所以编辑器里念出的
// 条件和保存后详情栏里念出的是同一句。行还没填完整就答 null，页面不显示。

import { describeTrigger, featureMeasure } from '../price-card-content';
import { isDecimal } from './validate';
import { remoteTiers } from './workbook';
import type { ConditionRow, FeeItemState } from './model';

type Doc = Record<string, unknown>;

function decimalOf(text: string): { coefficient: string; scale: number } | null {
  const trimmed = text.trim();
  if (!isDecimal(trimmed)) return null;
  const [whole, fraction = ''] = trimmed.split('.');
  return { coefficient: (whole + fraction).replace(/^0+(?=\d)/, ''), scale: fraction.length };
}

/** 一行条件 → 规范化文档里的判定；没填完整答 null。 */
export function conditionDoc(row: ConditionRow): Doc | null {
  const measure = featureMeasure(row.source);
  if (measure === undefined) return null;
  if (measure === 'category') {
    return row.value.trim() === '' ? null : { kind: 'PREDICATE', predicate: { source: row.source, operator: 'EQ', category: row.value.trim() } };
  }
  const value = decimalOf(row.value);
  if (value === null || row.operator === '' || row.unit === '') return null;
  const threshold = { value, unit: row.unit };
  const key = measure === 'length' ? 'lengthThreshold' : measure === 'volume' ? 'volumeThreshold' : 'weightThreshold';
  return { kind: 'PREDICATE', predicate: { source: row.source, operator: row.operator, [key]: threshold } };
}

export function conditionRowText(row: ConditionRow): string | null {
  const doc = conditionDoc(row);
  return doc === null ? null : describeTrigger(doc);
}

/** 整个触发条件的一句话；任何一行没填完整就答 null。 */
export function feeConditionText(fee: FeeItemState): string | null {
  const rows: ConditionRow[] =
    fee.kind === 'TABLE_REMOTE'
      ? remoteTiers(fee).map((tier, index) => ({ uid: `tier-${index}`, source: 'ADDRESS_TYPE', operator: 'EQ', value: tier, unit: '' }))
      : fee.conditions;
  if (rows.length === 0) return null;
  const docs = rows.map(conditionDoc);
  if (docs.some((doc) => doc === null)) return null;
  const operands = docs as Doc[];
  if (operands.length === 1) return describeTrigger(operands[0]);
  const kind = fee.kind === 'TABLE_REMOTE' ? 'ANY_OF' : fee.combine;
  return describeTrigger({ kind, operands });
}
