// 模板规格口（GET /pricing-price-card-template/spec）的读法。编辑器的列标题、必填性、可选值都从规格取，这里只放「怎么取」。
// 唯一手写的映射是价表类型 → 价表行所在的表：规格只描述列，不描述哪一族用哪张表（ADR-0349 决定七）。

import type { PriceCardSpecColumn, PriceCardSpecSheet, PriceCardTemplateSpec } from '../api';

export const sheetNames = {
  card: 'card',
  tables: 'tables',
  ratesWeightZone: 'rates_weight_zone',
  ratesFirstContinue: 'rates_first_continue',
  ratesUnitPrice: 'rates_unit_price',
  weightRounding: 'weight_rounding',
  fixedCharges: 'fixed_charges',
  surcharges: 'surcharges',
  conditions: 'conditions',
  calculations: 'calculations',
  chargeDependencies: 'charge_dependencies',
  totalBounds: 'total_bounds',
  referenceSeries: 'reference_series',
  referenceCatalogues: 'reference_catalogues',
} as const;

/** 价表类型对应的价表行表。 */
export const familySheet: Readonly<Record<string, string>> = {
  WEIGHT_ZONE: sheetNames.ratesWeightZone,
  FIRST_CONTINUE: sheetNames.ratesFirstContinue,
  UNIT_PRICE: sheetNames.ratesUnitPrice,
};

/** 价表行里由编辑器生成、不让运营填的列。 */
const generatedRateColumns = new Set(['tableId', 'entryId']);

export function sheetOf(spec: PriceCardTemplateSpec, name: string): PriceCardSpecSheet | undefined {
  return spec.sheets.find((sheet) => sheet.name === name);
}

export function columnOf(spec: PriceCardTemplateSpec, sheet: string, key: string): PriceCardSpecColumn | undefined {
  return sheetOf(spec, sheet)?.columns.find((column) => column.key === key);
}

/** 一列的可取值（规格口的 allowedValues）；没有封闭取值答空数组。 */
export function allowedValues(spec: PriceCardTemplateSpec, sheet: string, key: string): string[] {
  return columnOf(spec, sheet, key)?.allowedValues ?? [];
}

/** 一列的中文标题；规格里没有这一列时退回列键。 */
export function columnTitle(spec: PriceCardTemplateSpec, sheet: string, key: string): string {
  return columnOf(spec, sheet, key)?.title ?? key;
}

/** 某价表类型的价表行要运营填的列。 */
export function rateColumns(spec: PriceCardTemplateSpec, family: string): PriceCardSpecColumn[] {
  const sheet = familySheet[family];
  if (sheet === undefined) return [];
  return (sheetOf(spec, sheet)?.columns ?? []).filter((column) => !generatedRateColumns.has(column.key));
}

/** 价表行里不是数字的列（分区名 / 档位名）。 */
export function isTextColumn(key: string): boolean {
  return key === 'zone';
}

/** card 表的字段清单（规格里 layout 为 FIELDS 的那张表的「列」）。 */
export function cardFieldKeys(spec: PriceCardTemplateSpec): string[] {
  return (sheetOf(spec, sheetNames.card)?.columns ?? []).map((column) => column.key);
}

/**
 * 价表行里一列带的是重量还是金额，给表头补「（千克）」「（CNY）」用。规格只给列的中文标题，不说列的量纲，
 * 这里按列键认；认不出的列不补。
 */
export function rateColumnKind(key: string): 'weight' | 'money' | undefined {
  switch (key) {
    case 'minimum':
    case 'maximum':
    case 'firstWeight':
    case 'step':
      return 'weight';
    case 'amount':
    case 'firstAmount':
    case 'stepAmount':
    case 'amountPerUnit':
      return 'money';
    default:
      return undefined;
  }
}
