// 国家 / 地区码与币种码的译名（ADR-0335 决定七）：「CN」显示成「中国（CN）」，「CNY」显示成「人民币（CNY）」。
// 名称来自产品内置的基础数据目录（GET /reference-data/countries、/currencies），页面只改显示，不改数据，也不改请求或表单里的值。
//
// 查不到原样显示：目录读不回来（渠道未配置、没有授予、依赖故障）、目录之外的码（合成数据、历史数据里的形状合法码）、
// 空码，都按码本身示出——一次读不到不能被当成“目录里没有这个码”，所以读不回来时也不缓存（见 use-refdata-names.ts）。
// 本文件是纯函数，node:test 钉着；取数与订阅在 use-refdata-names.ts。

import type { ApiResult, CountryListResponseBody, CurrencyListResponseBody } from './reference-data/api';

/** 码 → 中文名。 */
export interface RefdataNames {
  readonly countries: ReadonlyMap<string, string>;
  readonly currencies: ReadonlyMap<string, string>;
}

export const emptyRefdataNames: RefdataNames = { countries: new Map(), currencies: new Map() };

/** 目前已读回的译名；读回之前与读不到时是空表。countryLabel / currencyLabel 不传名表时查它，调用方不必把名表一路传下去。 */
let currentNames: RefdataNames = emptyRefdataNames;

export function currentRefdataNames(): RefdataNames {
  return currentNames;
}

export function setCurrentRefdataNames(names: RefdataNames): void {
  currentNames = names;
}

function labelOf(names: ReadonlyMap<string, string>, code: string): string {
  const name = names.get(code);
  return name === undefined || name === '' ? code : `${name}（${code}）`;
}

/** 国家 / 地区码 → 「中国（CN）」；查不到（含空码）原样返回。 */
export function countryLabel(code: string, names: RefdataNames = currentNames): string {
  return labelOf(names.countries, code);
}

/** 币种码 → 「人民币（CNY）」；查不到（含空码）原样返回。 */
export function currencyLabel(code: string, names: RefdataNames = currentNames): string {
  return labelOf(names.currencies, code);
}

export interface RefdataNamesResult {
  names: RefdataNames;
  /** 两本目录都读回了才算完整；不完整的不进缓存，下次挂载再问。 */
  complete: boolean;
}

/** 把两次读口的答复折成名表：哪本读回了就用哪本，读不回的那本留空（页面按码显示）。 */
export function refdataNamesFromAnswers(
  countries: ApiResult<CountryListResponseBody>,
  currencies: ApiResult<CurrencyListResponseBody>,
): RefdataNamesResult {
  const countryNames = new Map<string, string>();
  const currencyNames = new Map<string, string>();
  if (countries.kind === 'outcome') for (const row of countries.body.countries) countryNames.set(row.code, row.nameZh);
  if (currencies.kind === 'outcome') for (const row of currencies.body.currencies) currencyNames.set(row.code, row.nameZh);
  return {
    names: { countries: countryNames, currencies: currencyNames },
    complete: countries.kind === 'outcome' && currencies.kind === 'outcome',
  };
}
