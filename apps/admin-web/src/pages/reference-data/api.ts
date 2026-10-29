// 基础数据目录查阅（GET /reference-data/countries、/currencies、/units）。形状镜像
// internal/platform/refdata/refdatahttp 的读口契约（ADR-0335 决定六）：产品内置的只读目录，不分租户，不分页。
// 页面与译名显示件都整表取回、在前端检索与分组，服务端的 ?q= / ?region= / ?category= 留给表单候选之类的后续消费者。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

export interface CountryRecord {
  code: string;
  nameZh: string;
  nameEn: string;
  /** 封闭七词（大洲），译法在词典的 refdata-region 册。 */
  region: string;
}

export interface CurrencyRecord {
  code: string;
  nameZh: string;
  nameEn: string;
  symbol: string;
  /** 最小币单位的小数位，0..3。 */
  minorUnitScale: number;
}

export interface UnitRecord {
  code: string;
  nameZh: string;
  /** 封闭四词（类别），译法在词典的 refdata-unit-category 册。 */
  category: string;
  baseUnit: string;
  /** 1 个本单位等于多少基准单位，精确十进制文本，原样示出、不经浮点。 */
  factorToBase: string;
}

export interface CountryListResponseBody {
  outcome: 'COUNTRIES_LISTED';
  countries: CountryRecord[];
}

export interface CurrencyListResponseBody {
  outcome: 'CURRENCIES_LISTED';
  currencies: CurrencyRecord[];
}

export interface UnitListResponseBody {
  outcome: 'UNITS_LISTED';
  units: UnitRecord[];
}

export function listCountries(): Promise<ApiResult<CountryListResponseBody>> {
  return exchangeMasterData<CountryListResponseBody>('/reference-data/countries');
}

export function listCurrencies(): Promise<ApiResult<CurrencyListResponseBody>> {
  return exchangeMasterData<CurrencyListResponseBody>('/reference-data/currencies');
}

export function listUnits(): Promise<ApiResult<UnitListResponseBody>> {
  return exchangeMasterData<UnitListResponseBody>('/reference-data/units');
}
