// 单据编号规则册四口（ADR-0355 决定六）。形状镜像 internal/platform/docnumber/docnumberhttp 的包注释（那里是契约，
// 改那里须同改这里）：读口与预览口挂主数据与运营查阅读，两个登记口挂登记册配置写。租户只取自认证信封，载荷里不带。
//
// 规则册是编号技术组件的配置，不属任何限界上下文（CONTEXT-MAP「公司级技术组件」）；单据号本身归各单据所属的上下文。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

export type RuleSource = 'PRODUCT_DEFAULT' | 'TENANT_RULE';

/** 日期按谁的时区出（docnumber.DateTimeZoneSource）。 */
export type DateTimeZoneSource = 'ISSUER_BUSINESS_TIME_ZONE' | 'TENANT_NUMBERING_TIME_ZONE';

export type NumberingTimeZoneStatus = 'CONFIGURED' | 'NOT_CONFIGURED';

export interface RuleVersionRecord {
  /** 只在登记口的答复里有；读口的历次版本挂在类型下面，不重复带。 */
  documentType?: string;
  template: string;
  effectiveFrom: string;
  /** 「OPERATOR/<发行方>#<sub>」或受控登记口的「CONTROLLED-CLI/…」。 */
  registeredBy: string;
  registeredAt: string;
}

export interface TimeZoneVersionRecord {
  /** IANA 时区名，租户取值。 */
  timeZone: string;
  effectiveFrom: string;
  registeredBy: string;
  registeredAt: string;
}

export interface NumberingTimeZoneRecord {
  status: NumberingTimeZoneStatus;
  /** 以下各格只在 CONFIGURED 时在场。 */
  timeZone?: string;
  effectiveFrom?: string;
  registeredBy?: string;
  registeredAt?: string;
}

export interface DocumentTypeRecord {
  documentType: string;
  defaultTemplate: string;
  source: RuleSource;
  /** 此刻生效的模板原文（租户版或产品默认）。 */
  template: string;
  /** 只随租户版本在场；产品默认没有生效时刻。 */
  effectiveFrom?: string;
  resetPeriod: string;
  dateTimeZone: {
    source: DateTimeZoneSource | string;
    numberingTimeZoneStatus: NumberingTimeZoneStatus;
    numberingTimeZone?: string;
  };
  /** 按生效模板、单据编号时区与第一个流水渲染的样例号；单据编号时区未配置时缺席。 */
  sample?: string;
  /** 本公司这一类的历次规则版本，按生效时刻升序。 */
  versions: RuleVersionRecord[];
}

export interface DocumentNumberingReadBody {
  outcome: 'DOCUMENT_NUMBERING_READ';
  /** 服务端答这一问的时刻：「此刻生效」都按它判。 */
  at: string;
  numberingTimeZone: NumberingTimeZoneRecord;
  numberingTimeZoneVersions: TimeZoneVersionRecord[];
  documentTypes: DocumentTypeRecord[];
}

export function readDocumentNumbering(): Promise<ApiResult<DocumentNumberingReadBody>> {
  return exchangeMasterData<DocumentNumberingReadBody>('/document-numbering/rules');
}

export interface PreviewPayload {
  documentType: string;
  template: string;
  /** 不给取租户的单据编号时区。 */
  timeZone?: string;
  /** 样例时刻；不给取此刻。登记新规则时传生效时刻，样例就是生效那一刻的号。 */
  at?: string;
}

export interface PreviewOverlapRecord {
  source: RuleSource;
  template: string;
  /** 产品默认没有生效时刻。 */
  effectiveFrom?: string;
}

/** 预览口的三种 200 答复。拒因放在 200 的 reason 里：边填边预览时半截格式被拒是常态，不是请求错了。 */
export type PreviewBody =
  | {
      outcome: 'PREVIEW_RENDERED';
      sample: string;
      timeZone: string;
      at: string;
      resetPeriod: string;
      overlaps: PreviewOverlapRecord[];
    }
  | { outcome: 'PREVIEW_REJECTED'; reason: string; resetPeriod?: string }
  | { outcome: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'; resetPeriod: string };

/** 预览不写册、不占号（同试算挂查阅读面），所以边填边调。 */
export function previewDocumentNumber(payload: PreviewPayload): Promise<ApiResult<PreviewBody>> {
  return postMasterData<PreviewBody>('/document-numbering/previews', payload);
}

export interface RuleRegistrationPayload {
  documentType: string;
  template: string;
  effectiveFrom: string;
}

export type RegistrationOutcome = 'VERSION_REGISTERED' | 'VERSION_EXISTING' | 'EFFECTIVE_FROM_TAKEN';

export interface RuleRegistrationBody {
  outcome: RegistrationOutcome;
  version: RuleVersionRecord;
}

export function registerRuleVersion(payload: RuleRegistrationPayload): Promise<ApiResult<RuleRegistrationBody>> {
  return postMasterData<RuleRegistrationBody>('/document-numbering/rule-registrations', payload);
}

export interface TimeZoneRegistrationPayload {
  timeZone: string;
  effectiveFrom: string;
}

export interface TimeZoneRegistrationBody {
  outcome: RegistrationOutcome;
  version: TimeZoneVersionRecord;
}

export function registerNumberingTimeZone(payload: TimeZoneRegistrationPayload): Promise<ApiResult<TimeZoneRegistrationBody>> {
  return postMasterData<TimeZoneRegistrationBody>('/document-numbering/time-zone-registrations', payload);
}
