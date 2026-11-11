// 业务参与方身份登记表单的纯逻辑（票 admin-web-group-legal-entities/10；ADR-0101 决定八自裁：格少、低频、
// 无矩阵——直接逐字段表单，形状照票 02 的 legal-entity-form.ts）。全部是纯函数，node:test 钉着；组件
// BusinessPartyRegistrationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）。参与方标识与名称是否为空、
// 修订号连不连续，一律原样送上去让服务端逐格答；这里只做编码层的事：修订号编成 JSON 整数、墙钟时刻换成
// RFC 3339、可缺的键缺席而不是零值。
//
// **各格不裁首尾空白。** 这一点与票 02 的法人表单相反、与服务端一致：isolated_write_intake.go 的
// businessPartyDocument 注释写明字段不做裁切、与受控 CLI 同判，表单那层裁了会让 " X" 与 "X" 在这一口成为
// 同一身份、在 CLI 却是两个。修订号那一格例外——它编成的是数，不是身份串，周围空白不构成第二个值。
//
// **载荷不带租户格。** 镜像受控 CLI `parcel-commercial register-parties` 输入里 `businessParties` 数组的一项，
// 去掉整批的 `tenantId`：隔离 Intake 对键在场即拒、含 null（refuseSelfReportedTenant）。

import type { BusinessPartyIdentificationRecord, BusinessPartyRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';
import { revisionOf, suggestedNextRevision } from './registration-form';

/** 一行注册号：类型码取自注册号类型目录的身份层，号原样。 */
export interface PartyNumberRow {
  typeCode: string;
  number: string;
}

/** 一行外部系统编码；purpose 未选是空串。 */
export interface PartyExternalReferenceRow {
  system: string;
  purpose: string;
  code: string;
}

export interface BusinessPartyDraft {
  partyId: string;
  name: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟），留空即缺席。 */
  effectiveFrom: string;
  /** 识别信息（ADR-0367）各格可缺：注册国家空串即没给；三格都空的行是没填的空行，组载荷时不带。 */
  registrationCountry: string;
  lifetimeNumbers: PartyNumberRow[];
  /** 身份更正依据；只在改了或去掉已登的号时要填，空串即缺席。 */
  identityCorrectionBasis: string;
  externalReferences: PartyExternalReferenceRow[];
}

/** 键名逐字取 businessPartyDocument 的 json 标签。 */
export interface BusinessPartyRegistrationItem {
  partyId: string;
  name: string;
  revision?: number;
  basis: string;
  effectiveFrom?: string;
  registrationCountry?: string;
  lifetimeRegistrationNumbers?: PartyNumberRow[];
  identityCorrectionBasis?: string;
  externalReferences?: PartyExternalReferenceRow[];
}

export interface BusinessPartyRegistrationPayload {
  businessParties: BusinessPartyRegistrationItem[];
}

export function emptyBusinessPartyDraft(): BusinessPartyDraft {
  return {
    partyId: '',
    name: '',
    revision: '',
    basis: '',
    effectiveFrom: '',
    registrationCountry: '',
    lifetimeNumbers: [],
    identityCorrectionBasis: '',
    externalReferences: [],
  };
}

/**
 * 册上这一行的识别信息摆回草稿：更正名称时照带原有的号与编码——一笔修订是整份内容，不带即是去掉（ADR-0367 决定一）。
 * 更正依据不带：它说的是「这一笔改了号」，不是那一行的属性。
 */
export function identificationDraftOf(
  row: BusinessPartyIdentificationRecord,
): Pick<BusinessPartyDraft, 'registrationCountry' | 'lifetimeNumbers' | 'identityCorrectionBasis' | 'externalReferences'> {
  const registered = row.identityLayerRegistered === true;
  return {
    registrationCountry: registered ? (row.registrationCountry ?? '') : '',
    lifetimeNumbers: registered ? (row.lifetimeRegistrationNumbers ?? []).map((entry) => ({ ...entry })) : [],
    identityCorrectionBasis: '',
    externalReferences: (row.externalReferences ?? []).map((entry) => ({ ...entry })),
  };
}

const filledNumbers = (rows: readonly PartyNumberRow[]) => rows.filter((row) => row.typeCode !== '' || row.number !== '');
const filledReferences = (rows: readonly PartyExternalReferenceRow[]) =>
  rows.filter((row) => row.system !== '' || row.purpose !== '' || row.code !== '');

function numbersKey(country: string, rows: readonly PartyNumberRow[]): string {
  return JSON.stringify([country, [...rows].map((row) => [row.typeCode, row.number]).sort()]);
}

/**
 * 草稿上的号与册上这一行的号是不是不一样了（国家、或整组号，与录入次序无关）。册上没登号时答 false——第一次补登不是更正。
 * 这只是给表单决定「要不要摆出身份更正依据那一格」的呈现判断；要不要更正依据，由服务端答。
 */
export function registeredNumbersChanged(row: BusinessPartyIdentificationRecord, draft: BusinessPartyDraft): boolean {
  if (row.identityLayerRegistered !== true) return false;
  return (
    numbersKey(row.registrationCountry ?? '', row.lifetimeRegistrationNumbers ?? []) !==
    numbersKey(draft.registrationCountry, filledNumbers(draft.lifetimeNumbers))
  );
}

/** 草稿上的识别信息与册上这一行比有没有改动（号、或外部编码，与录入次序无关）。 */
export function identificationChanged(row: BusinessPartyIdentificationRecord, draft: BusinessPartyDraft): boolean {
  const before = identificationDraftOf(row);
  const referencesKey = (rows: readonly PartyExternalReferenceRow[]) =>
    JSON.stringify([...rows].map((entry) => [entry.system, entry.purpose, entry.code]).sort());
  return (
    numbersKey(before.registrationCountry, before.lifetimeNumbers) !==
      numbersKey(draft.registrationCountry, filledNumbers(draft.lifetimeNumbers)) ||
    referencesKey(before.externalReferences) !== referencesKey(filledReferences(draft.externalReferences))
  );
}

export interface IdentityCorrectionChange {
  field: string;
  before: string;
  after: string;
}

const numbersText = (rows: readonly PartyNumberRow[]) =>
  [...rows].map((row) => `${row.typeCode} ${row.number}`.trim()).sort().join('；');
const referencesText = (rows: readonly PartyExternalReferenceRow[]) =>
  [...rows].map((row) => `${[row.system, row.purpose].filter(Boolean).join(' · ')} ${row.code}`.trim()).sort().join('；');

/**
 * 「更正登记信息」保存前核对的那几行（票 business-party-redesign/11）：名称、注册国家 / 地区、注册号、外部系统编码，只列有改动的。
 * 号与编码各折成一段文字比，与录入次序无关，没填的空行不算——与 identificationChanged 判「有没有改动」同一个口径。码照原样列
 * （国家码、类型码），列的是将要送上去的值。
 */
export function identityCorrectionChanges(
  row: BusinessPartyIdentificationRecord,
  currentName: string,
  draft: BusinessPartyDraft,
): IdentityCorrectionChange[] {
  const before = identificationDraftOf(row);
  const rows: IdentityCorrectionChange[] = [
    { field: '名称', before: currentName, after: draft.name.trim() },
    { field: '注册国家 / 地区', before: before.registrationCountry, after: draft.registrationCountry.trim() },
    { field: '注册号', before: numbersText(before.lifetimeNumbers), after: numbersText(filledNumbers(draft.lifetimeNumbers)) },
    {
      field: '外部系统编码',
      before: referencesText(before.externalReferences),
      after: referencesText(filledReferences(draft.externalReferences)),
    },
  ];
  return rows.filter((entry) => entry.before !== entry.after);
}

/** 表单认领的 JSON 路径；一项一批，下标固定 0。 */
export const businessPartyFieldPaths = [
  'businessParties[0].partyId',
  'businessParties[0].name',
  'businessParties[0].revision',
  'businessParties[0].basis',
  'businessParties[0].effectiveFrom',
  'businessParties[0].registrationCountry',
  'businessParties[0].lifetimeRegistrationNumbers',
  'businessParties[0].identityCorrectionBasis',
  'businessParties[0].externalReferences',
] as const;

export type BusinessPartyFieldPath = (typeof businessPartyFieldPaths)[number];

/**
 * 草稿 → 载荷。身份串原样带（含首尾空白与空串——空串是服务端要点名的格）；修订号编不进正整数与生效时刻
 * 换不出来时**该键缺席**——不造一个假数顶上，由 businessPartyLocalProblems 在送之前拦。
 */
export function businessPartyPayloadOf(draft: BusinessPartyDraft, timeZone: string): BusinessPartyRegistrationPayload {
  const item: BusinessPartyRegistrationItem = {
    partyId: draft.partyId,
    name: draft.name,
    basis: draft.basis,
  };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.effectiveFrom.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.effectiveFrom, timeZone);
    if (moment !== null) item.effectiveFrom = moment;
  }
  // 识别信息：没填的格缺席，空行不带；填了的原样带——国家缺了号还在、号行只填一半，都送上去让服务端点名。
  if (draft.registrationCountry !== '') item.registrationCountry = draft.registrationCountry;
  const numbers = filledNumbers(draft.lifetimeNumbers);
  if (numbers.length > 0) item.lifetimeRegistrationNumbers = numbers.map((row) => ({ ...row }));
  if (draft.identityCorrectionBasis !== '') item.identityCorrectionBasis = draft.identityCorrectionBasis;
  const references = filledReferences(draft.externalReferences);
  if (references.length > 0) item.externalReferences = references.map((row) => ({ ...row }));
  return { businessParties: [item] };
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。只有编码层的：修订号编不进正整数、生效时刻换不成
 * RFC 3339。空格、标识是否已在册、修订是否连续都不在这里判，那是服务端的话。
 */
export function businessPartyLocalProblems(draft: BusinessPartyDraft, timeZone: string): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['businessParties[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.effectiveFrom.trim() !== '' && wallTimeToRfc3339(draft.effectiveFrom, timeZone) === null) {
    problems['businessParties[0].effectiveFrom'] = ['生效时刻要是可解析的日期时间'];
  }
  return problems;
}

/**
 * 修订号建议值（规则在 suggestedNextRevision，这里只交本册取键的字段）。查找按原串比而不裁空白，与载荷同判——载荷里
 * " X" 就是另一个身份，建议也得按它是新标识算，否则建议与送上去的东西说的不是同一个对象。
 */
export function suggestedBusinessPartyRevision(rows: readonly BusinessPartyRecord[] | null, partyId: string): number {
  return suggestedNextRevision(rows, partyId, (row) => row.partyId);
}
