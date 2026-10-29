// 下拉选项里需要「规格口 + 目录」合起来才算得出的两处。纯函数，页面只负责画。

import type { PriceCardTemplateSpec } from '../api';
import { addressNatureValueLabels } from '../../../vocabulary/price-card';
import { allowedValues, closedValuesWhen, sheetNames } from './spec';

export interface Choice {
  value: string;
  label: string;
}

/**
 * 条件行「类别」格的下拉选项：规格口说这一行（来源取当前值）的类别是封闭的就给选项，译法取词典、词典没有的取值以码原样显示；
 * 开放词汇（分区名、偏远档位、服务选项……）答 null，页面按文本画。
 * 「来源为地址性质时才封闭」是规格口的话（allowedValuesWhen），这里不认地址性质这个名字。
 */
export function conditionCategoryChoices(spec: PriceCardTemplateSpec, source: string): Choice[] | null {
  const closed = closedValuesWhen(spec, sheetNames.conditions, 'category', { source });
  if (closed === null) return null;
  return closed.map((value) => ({ value, label: addressNatureValueLabels[value] ?? value }));
}

export interface CurrencyChoices {
  /** 币种下拉的选项；两处都读不到时是 null，页面退回手填。 */
  options: Choice[] | null;
  /** 规格口有而目录没有的码 / 目录有而规格口没有的码：两处本应一致，不一致时以规格口为准。 */
  onlyInSpec: string[];
  onlyInCatalogue: string[];
}

/**
 * 币种下拉：可取的币种以规格口的 `tables.currency` 为准，名字取基础数据的币种目录。规格口没给币种取值（旧后端）时退回只用目录；
 * 目录读不到时用规格口的码，名字就是码本身。
 */
export function currencyChoices(
  spec: PriceCardTemplateSpec,
  catalogue: readonly { code: string; label: string }[] | null,
): CurrencyChoices {
  const fromSpec = allowedValues(spec, sheetNames.tables, 'currency');
  if (fromSpec.length === 0) {
    return { options: catalogue === null ? null : catalogue.map((entry) => ({ value: entry.code, label: entry.label })), onlyInSpec: [], onlyInCatalogue: [] };
  }
  const names = new Map((catalogue ?? []).map((entry) => [entry.code, entry.label]));
  return {
    options: fromSpec.map((code) => ({ value: code, label: names.get(code) ?? code })),
    onlyInSpec: catalogue === null ? [] : fromSpec.filter((code) => !names.has(code)),
    onlyInCatalogue: catalogue === null ? [] : catalogue.map((entry) => entry.code).filter((code) => !fromSpec.includes(code)),
  };
}
