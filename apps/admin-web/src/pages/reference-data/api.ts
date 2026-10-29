// 基础数据目录查阅（GET /reference-data/countries、/currencies、/units）。形状镜像
// internal/platform/refdata/refdatahttp 的读口契约（ADR-0335 决定六）：产品内置的只读目录，不分租户，不分页。
// 页面与译名显示件都整表取回、在前端检索与分组，服务端的 ?q= / ?region= / ?category= 留给表单候选之类的后续消费者。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';

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

// —— 启用范围（ADR-0345）——
// 读回：GET /reference-data/enablement?kind=COUNTRY|CURRENCY，答本公司册上登过的每个码此刻的状态与全部历史；册上没有行的码
// 不在结果里（未配置由缺席表达）。登记：POST /reference-data/enablement-registrations，启用与停用同是追加一笔。
// 形状镜像 internal/partycommercial/adapters/http 的 reference_enablement.go。


export type EnablementKind = 'COUNTRY' | 'CURRENCY';

/** 此刻的状态：封闭三词。NOT_CONFIGURED 只出现在读回的 standing 上，不是可登记的状态。 */
export type EnablementStanding = 'ENABLED' | 'DISABLED' | 'NOT_CONFIGURED';

export interface EnablementRevisionRecord {
  revision: number;
  /** 登记时说的话：ENABLED 或 DISABLED。 */
  status: 'ENABLED' | 'DISABLED';
  effectiveFrom: string;
  /** 登记者写下的来源单据；没写就缺席。 */
  source?: string;
  /** 登记责任方：取自认证信封，“发行方#sub”。 */
  registeredBy: string;
  recordedAt: string;
}

export interface EnablementRecord {
  code: string;
  standing: EnablementStanding;
  /** 此刻生效的那一行的生效时点；没有生效的行时缺席。 */
  effectiveFrom?: string;
  /** 册上最新的修订号；下一笔登记它加一。 */
  latestRevision: number;
  history: EnablementRevisionRecord[];
}

export interface EnablementListResponseBody {
  outcome: 'REFERENCE_ENABLEMENTS_LISTED';
  kind: EnablementKind;
  asOf: string;
  enablements: EnablementRecord[];
}

export function listEnablements(kind: EnablementKind): Promise<ApiResult<EnablementListResponseBody>> {
  return exchangeMasterData<EnablementListResponseBody>(`/reference-data/enablement?kind=${kind}`);
}

export interface EnablementRegistrationPayload {
  kind: EnablementKind;
  code: string;
  revision: number;
  status: 'ENABLED' | 'DISABLED';
  effectiveFrom: string;
  source?: string;
}

/** 登记口的封闭答复：REGISTERED（201）之外都是 200 的业务答复，理由在 refusalReason（未受理时）。 */
export interface EnablementAnswerBody {
  outcome: 'REGISTERED' | 'ALREADY_REGISTERED' | 'CONTENT_CONFLICT' | 'NOT_ACCEPTED';
  refusalReason?: 'CODE_NOT_IN_CATALOGUE' | 'REVISION_OUT_OF_SEQUENCE' | 'EFFECTIVE_BEFORE_PRIOR' | 'INPUT_NOT_WELL_FORMED';
  /** 散文，原样示出、不据此分支。 */
  cause?: string;
}

export function registerEnablement(payload: EnablementRegistrationPayload): Promise<ApiResult<EnablementAnswerBody>> {
  return postMasterData<EnablementAnswerBody>('/reference-data/enablement-registrations', payload);
}
