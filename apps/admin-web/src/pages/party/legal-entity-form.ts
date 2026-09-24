// 责任法人身份登记表单的纯逻辑（票 admin-web-group-legal-entities/02，身份层三格随票 legal-entity-profile/04 加入；
// ADR-0101 决定八自裁：格少、低频、无矩阵——直接逐字段表单）。全部是纯函数，node:test 钉着；组件
// LegalEntityRegistrationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）。参与方在不在册、届时
// 是否已生效、修订号连不连续、依据是否为空，一律原样送上去让服务端逐格答；这里只做编码层的事：
// 修订号编成 JSON 整数、墙钟时刻换成 RFC 3339、可缺的键缺席而不是空串。
//
// **载荷不带租户格。** 镜像受控 CLI `parcel-commercial register-parties` 输入里 `legalEntities` 数组的一项，
// 但去掉整批的 `tenantId`：在线 Intake 要把认证结果填进租户格、只从载荷取行内容（register_party_identity.go
// 包注释）；载荷里带上租户就是请求方自报租户。今天端点挂 UnconfiguredIntake{} 必答 403，这份形状没有服务端
// 消费者，真 Intake 落地时以它为准重谈，不把这里当已发布 Schema（api.ts 那段注释的原话）。

import type { GroupLegalEntityRecord, LifetimeRegistrationNumberRecord, RegistrationNumberTypeRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';
import { revisionOf, suggestedNextRevision } from './registration-form';

/** 终身注册号的一行草稿：类型码与号各是文本框原值。 */
export interface LifetimeNumberDraft {
  typeCode: string;
  number: string;
}

export interface LegalEntityDraft {
  legalEntityId: string;
  partyId: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟），留空即缺席。 */
  effectiveFrom: string;
  /** 注册国家 / 地区码原值（ADR-0145 决定一），留空即缺席。 */
  registrationCountry: string;
  /** 终身注册号逐行；类型与号都空的行当作没填。 */
  lifetimeNumbers: LifetimeNumberDraft[];
  /** 身份更正依据原值（ADR-0145 决定二：录错走更正修订并携带依据），留空即缺席。 */
  identityCorrectionBasis: string;
}

export interface LegalEntityRegistrationItem {
  legalEntityId: string;
  partyId: string;
  revision?: number;
  basis: string;
  effectiveFrom?: string;
  registrationCountry?: string;
  lifetimeRegistrationNumbers?: LifetimeRegistrationNumberRecord[];
  identityCorrectionBasis?: string;
}

export interface LegalEntityRegistrationPayload {
  legalEntities: LegalEntityRegistrationItem[];
}

export function emptyLegalEntityDraft(): LegalEntityDraft {
  return {
    legalEntityId: '',
    partyId: '',
    revision: '',
    basis: '',
    effectiveFrom: '',
    registrationCountry: '',
    lifetimeNumbers: [{ typeCode: '', number: '' }],
    identityCorrectionBasis: '',
  };
}

/** 表单认领的 JSON 路径，服务端逐格问题按它点名；一项一批，下标固定 0。 */
export const legalEntityFieldPaths = [
  'legalEntities[0].legalEntityId',
  'legalEntities[0].partyId',
  'legalEntities[0].revision',
  'legalEntities[0].basis',
  'legalEntities[0].effectiveFrom',
  'legalEntities[0].registrationCountry',
  'legalEntities[0].lifetimeRegistrationNumbers',
  'legalEntities[0].identityCorrectionBasis',
] as const;

export type LegalEntityFieldPath = (typeof legalEntityFieldPaths)[number];

/**
 * 草稿 → 载荷。各格去首尾空白后原样带（空串照送，那是服务端要点名的格）；修订号编不进正整数
 * 与生效时刻换不出来时**该键缺席**——不造一个假数顶上，由 legalEntityLocalProblems 在送之前拦。
 */
export function legalEntityPayloadOf(draft: LegalEntityDraft, timeZone: string): LegalEntityRegistrationPayload {
  const item: LegalEntityRegistrationItem = {
    legalEntityId: draft.legalEntityId.trim(),
    partyId: draft.partyId.trim(),
    basis: draft.basis.trim(),
  };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.effectiveFrom.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.effectiveFrom, timeZone);
    if (moment !== null) item.effectiveFrom = moment;
  }
  // 身份层三格留空即**键缺席**，不送空串：Intake 对这三键「给了就得合形」，空串会被当成给了而答形状错；缺席才轮到
  // 用例按 ADR-0145 决定一答「未受理」并点名缺的是哪格——那才是操作者要看的答案。
  const country = draft.registrationCountry.trim();
  if (country !== '') item.registrationCountry = country;
  const numbers = filledNumberRows(draft.lifetimeNumbers);
  if (numbers.length > 0) item.lifetimeRegistrationNumbers = numbers;
  const correction = draft.identityCorrectionBasis.trim();
  if (correction !== '') item.identityCorrectionBasis = correction;
  return { legalEntities: [item] };
}

/** 去首尾空白后，类型与号至少一格有字的行；都空的行当作没填。半填的行照带，由 legalEntityLocalProblems 在送之前拦。 */
function filledNumberRows(rows: readonly LifetimeNumberDraft[]): LifetimeRegistrationNumberRecord[] {
  return rows
    .map((row) => ({ typeCode: row.typeCode.trim(), number: row.number.trim() }))
    .filter((row) => row.typeCode !== '' || row.number !== '');
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。只有三种：修订号编不进正整数、
 * 生效时刻换不成 RFC 3339、终身注册号有一行只填了类型或只填了号（Intake 收的每一项两格都得有，半行编不成一项）。
 * 空格、引用是否在册、国家在不在目录里、号合不合格式、修订是否连续都不在这里判，那是服务端的话。
 */
export function legalEntityLocalProblems(draft: LegalEntityDraft, timeZone: string): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['legalEntities[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.effectiveFrom.trim() !== '' && wallTimeToRfc3339(draft.effectiveFrom, timeZone) === null) {
    problems['legalEntities[0].effectiveFrom'] = ['生效时刻要是可解析的日期时间'];
  }
  const halfRows = draft.lifetimeNumbers
    .map((row, index) => ({ index, typeCode: row.typeCode.trim(), number: row.number.trim() }))
    .filter((row) => (row.typeCode === '') !== (row.number === ''));
  if (halfRows.length > 0) {
    problems['legalEntities[0].lifetimeRegistrationNumbers'] = halfRows.map(
      (row) => `第 ${row.index + 1} 行：类型与号要成对填`,
    );
  }
  return problems;
}

/** 选单一项；与 PublicationFormFields 的 PickerOption 同形，纯逻辑这层不去引 .tsx。 */
export interface IdentityCatalogueOption {
  value: string;
  label: string;
}

/**
 * 注册国家 / 地区候选：目录里登过身份层类型的国家码，去重、按码排序。不按状态过滤——表单不裁，停用与否由服务端
 * 登记时按目录判。
 */
export function identityCountryOptions(types: readonly RegistrationNumberTypeRecord[]): IdentityCatalogueOption[] {
  const codes = new Set(types.filter((type) => type.layer === 'IDENTITY').map((type) => type.countryCode));
  return [...codes].sort().map((code) => ({ value: code, label: code }));
}

/**
 * 某国家 / 地区的终身注册号类型候选：只列身份层（资料层的号不收进身份，ADR-0145 决定一），国家码去首尾空白后比。
 * 已停用的类型照样列、括注「已停用」：候选是给人看的，拒不拒由服务端判。
 */
export function identityTypeOptions(
  types: readonly RegistrationNumberTypeRecord[],
  country: string,
): IdentityCatalogueOption[] {
  const code = country.trim();
  return types
    .filter((type) => type.layer === 'IDENTITY' && type.countryCode === code)
    .map((type) => ({
      value: type.typeCode,
      label: `${type.typeCode} · ${type.typeName}${type.deactivatedAt !== undefined ? '（已停用）' : ''}`,
    }));
}

/**
 * 修订号建议值（规则在 suggestedNextRevision，这里只交本册取键的字段）。标识先裁首尾空白再找：本册的载荷裁空白
 * （legalEntityPayloadOf，票 02 验收过、legal-entity-form.test.ts 钉着），建议得与送上去的串说同一个对象。裁在这一层
 * 而不进共用体——其余各册不裁，是服务端各口判据的差别，不是共用体该替谁定的；要统一先改票 02 的判据与用例（票 13 第 3 条
 * 那半不做的理由）。
 */
export function suggestedRevision(rows: readonly GroupLegalEntityRecord[] | null, legalEntityId: string): number {
  return suggestedNextRevision(rows, legalEntityId.trim(), (row) => row.legalEntityId);
}
