// 公司主体详情「公司资料」区的纯逻辑（票 legal-entity-profile/04 第 2 项；ADR-0145 决定三、五、六）：登记表单的草稿 → 载荷、
// 本地编码层问题、修订号建议、资料状态判读，以及表单预填与保存前差异预览要的两个方向的折算（票 company-subject-ux/04）。
// 全部是纯函数，node:test 钉着；组件 LegalEntityProfileSection.tsx 只负责摆。
//
// **不算校验结论**（票面「不做」）：公司主体在不在册、地址国家与身份上的注册国家对不对得上、税号合不合目录、修订连不连续，
// 一律送上去让服务端答。本地只多一类「必填未填」的编码层提示（legalEntityProfileMissingFields），那是在说格空着，不是在判
// 领域规则。载荷镜像受控 CLI `register-legal-entity-profiles` 的一项、不带租户格——理由同法人登记表单（legal-entity-form.ts 文件头）。

import type {
  ContactPurpose,
  ContactSocialAccountRecord,
  LegalEntityRepresentativeRecord,
  LegalEntityContactRecord,
  LegalEntityProfileContent,
  LegalEntityProfileResolutionResponseBody,
  LegalEntityProfileRevisionRecord,
  RegisteredAddressRecord,
  RegistrationNumberTypeRecord,
  TaxRegistrationNumberRecord,
} from './api';
import { wallTimeToRfc3339 } from '../moment';
import { revisionOf } from './registration-form';

export interface TaxNumberDraft {
  typeCode: string;
  number: string;
}

export const contactPurposeLabels: Record<ContactPurpose, string> = {
  GENERAL: '日常联系',
  BUSINESS: '商务',
  OPERATIONS: '运营',
  FINANCE: '财务',
  LEGAL: '法务',
};

export interface ContactDraft {
  name: string;
  email: string;
  phone: string;
  department?: string;
  position?: string;
  purposes?: ContactPurpose[];
  primaryFor?: ContactPurpose[];
  socialAccounts?: ContactSocialAccountRecord[];
}

export interface LegalEntityProfileDraft {
  /** 对象页上那个公司主体，不是输入格：资料挂在哪个主体上由页面定。 */
  legalEntityId: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值，可以在未来：资料可以登记未来生效的修订（ADR-0145 决定三）。留空即缺席。 */
  effectiveFrom: string;
  addressCountry: string;
  /** 注册地址逐行原文，一行一段；空行不送。 */
  addressLines: string;
  taxNumbers: TaxNumberDraft[];
  /** 开票抬头；留空即这笔不带开票资料（键缺席）——空串服务端照样拒，不当缺席。 */
  invoiceTitle: string;
  contacts: ContactDraft[];
  representativeKind: '' | LegalEntityRepresentativeRecord['kind'];
  representativeName: string;
  officeCountry: string;
  officeLines: string;
  correspondenceCountry: string;
  correspondenceLines: string;
}

export interface LegalEntityProfileItem {
  legalEntityId: string;
  revision?: number;
  basis: string;
  effectiveFrom?: string;
  registeredAddress?: RegisteredAddressRecord;
  taxRegistrationNumbers?: TaxRegistrationNumberRecord[];
  invoiceTitle?: string;
  contacts?: LegalEntityContactRecord[];
  representative?: LegalEntityRepresentativeRecord;
  officeAddress?: RegisteredAddressRecord;
  correspondenceAddress?: RegisteredAddressRecord;
}

export interface LegalEntityProfilePayload {
  profiles: LegalEntityProfileItem[];
}

export function emptyLegalEntityProfileDraft(legalEntityId: string): LegalEntityProfileDraft {
  return {
    legalEntityId,
    revision: '',
    basis: '',
    effectiveFrom: '',
    addressCountry: '',
    addressLines: '',
    taxNumbers: [],
    invoiceTitle: '',
    contacts: [],
    representativeKind: '',
    representativeName: '',
    officeCountry: '',
    officeLines: '',
    correspondenceCountry: '',
    correspondenceLines: '',
  };
}

/**
 * 草稿 → 载荷。各格去首尾空白后原样带；可缺的键留空即缺席、不送空串（Intake 对它们「给了就得立得住」，空串会被当成
 * 给了而答形状错；缺席才轮到用例按 ADR-0145 答「未受理」并点名）。注册地址国家与各行全空才算没给，填了一部分照带。
 */
export function legalEntityProfilePayloadOf(
  draft: LegalEntityProfileDraft,
  timeZone: string,
): LegalEntityProfilePayload {
  const item: LegalEntityProfileItem = { legalEntityId: draft.legalEntityId.trim(), basis: draft.basis.trim() };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.effectiveFrom.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.effectiveFrom, timeZone);
    if (moment !== null) item.effectiveFrom = moment;
  }
  const country = draft.addressCountry.trim();
  const lines = draft.addressLines
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '');
  if (country !== '' || lines.length > 0) item.registeredAddress = { country, lines };
  const taxes = draft.taxNumbers
    .map((row) => ({ typeCode: row.typeCode.trim(), number: row.number.trim() }))
    .filter((row) => row.typeCode !== '' || row.number !== '');
  if (taxes.length > 0) item.taxRegistrationNumbers = taxes;
  const title = draft.invoiceTitle.trim();
  if (title !== '') item.invoiceTitle = title;
  const contacts = draft.contacts.flatMap((row) => {
    const contact: LegalEntityContactRecord = { name: row.name.trim() };
    if (row.email.trim()) contact.email = row.email.trim();
    if (row.phone.trim()) contact.phone = row.phone.trim();
    if (row.department?.trim()) contact.department = row.department.trim();
    if (row.position?.trim()) contact.position = row.position.trim();
    if (row.purposes?.length) contact.purposes = [...row.purposes];
    if (row.primaryFor?.length) contact.primaryFor = [...row.primaryFor];
    const accounts = (row.socialAccounts ?? []).map(({ platform, account }) => ({ platform: platform.trim(), account: account.trim() }));
    if (accounts.length) contact.socialAccounts = accounts;
    return Object.values(contact).some((value) => (Array.isArray(value) ? value.length > 0 : value !== ''))
      ? [contact]
      : [];
  });
  if (contacts.length > 0) item.contacts = contacts;
  if (draft.representativeKind && draft.representativeName.trim()) {
    item.representative = { kind: draft.representativeKind, name: draft.representativeName.trim() };
  }
  const office = addressPayload(draft.officeCountry, draft.officeLines);
  const correspondence = addressPayload(draft.correspondenceCountry, draft.correspondenceLines);
  if (office) item.officeAddress = office;
  if (correspondence) item.correspondenceAddress = correspondence;
  return { profiles: [item] };
}

/** 表单里一格「必填未填」：路径是那一格认领问题的 JSON 路径，标签给按钮旁的缺项清单。 */
export interface ProfileMissingField {
  path: string;
  label: string;
}

/**
 * 必填未填的格，按表单从上到下的次序：注册地址国家 / 地区与地址、各联系人的姓名、来源单据、生效时间。只看格空没空，
 * 国家对不对得上身份、时间连不连续仍由服务端答。联系人行整行空着也算：行是操作者加的，格上标着「姓名 *」，静默丢掉
 * 会让人以为登上去了。
 */
export function legalEntityProfileMissingFields(draft: LegalEntityProfileDraft): ProfileMissingField[] {
  const missing: ProfileMissingField[] = [];
  if (draft.addressCountry.trim() === '') missing.push({ path: 'profiles[0].registeredAddress', label: '注册地址国家 / 地区' });
  if (draft.addressLines.trim() === '') missing.push({ path: 'profiles[0].registeredAddress.lines', label: '注册地址' });
  draft.contacts.forEach((row, index) => {
    if (row.name.trim() === '') missing.push({ path: `profiles[0].contacts[${index}]`, label: `联系人 ${index + 1} 姓名` });
  });
  if (draft.basis.trim() === '') missing.push({ path: 'profiles[0].basis', label: '来源单据' });
  if (draft.effectiveFrom.trim() === '') missing.push({ path: 'profiles[0].effectiveFrom', label: '生效时间' });
  return missing;
}

/** 把缺项并进问题表，让各格在自己底下也显出来；只在操作者点过保存之后才并，之前空着的格不该一片红。 */
export function problemsWithMissing(
  problems: Record<string, string[]>,
  missing: readonly ProfileMissingField[],
): Record<string, string[]> {
  const merged: Record<string, string[]> = { ...problems };
  for (const field of missing) merged[field.path] = [...(merged[field.path] ?? []), '必填未填'];
  return merged;
}

/** 本地提示编码、成对输入与联系人分工问题；是否受理仍由服务端领域规则裁决。 */
export function legalEntityProfileLocalProblems(
  draft: LegalEntityProfileDraft,
  timeZone: string,
): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['profiles[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.effectiveFrom.trim() !== '' && wallTimeToRfc3339(draft.effectiveFrom, timeZone) === null) {
    problems['profiles[0].effectiveFrom'] = ['生效时刻要是可解析的日期时间'];
  }
  const halfRows = draft.taxNumbers
    .map((row, index) => ({ index, typeCode: row.typeCode.trim(), number: row.number.trim() }))
    .filter((row) => (row.typeCode === '') !== (row.number === ''));
  if (halfRows.length > 0) {
    problems['profiles[0].taxRegistrationNumbers'] = halfRows.map((row) => `第 ${row.index + 1} 行：类型与号要成对填`);
  }
  if (Boolean(draft.representativeKind) !== Boolean(draft.representativeName.trim())) {
    problems['profiles[0].representative'] = ['身份类型与姓名需要成对填写'];
  }
  for (const [key, country, lines] of [
    ['officeAddress', draft.officeCountry, draft.officeLines],
    ['correspondenceAddress', draft.correspondenceCountry, draft.correspondenceLines],
  ]) {
    if (Boolean(country.trim()) !== Boolean(lines.trim()))
      problems[`profiles[0].${key}`] = ['国家 / 地区与地址需要一起填写'];
  }
  const primary = new Set<ContactPurpose>();
  for (const [index, row] of draft.contacts.entries()) {
    const path = `profiles[0].contacts[${index}]`;
    if (
      !row.name.trim() &&
      (row.email.trim() ||
        row.phone.trim() ||
        row.department?.trim() ||
        row.position?.trim() ||
        row.purposes?.length ||
        row.primaryFor?.length ||
        row.socialAccounts?.length)
    ) {
      problems[path] = ['请填写联系人姓名'];
    }
    const seenAccounts = new Set<string>();
    for (const [accountIndex, social] of (row.socialAccounts ?? []).entries()) {
      const socialPath = `${path}.socialAccounts[${accountIndex}]`;
      const platform = social.platform.trim();
      const account = social.account.trim();
      const key = JSON.stringify([platform.toLowerCase(), account]);
      if (!platform || !account) problems[socialPath] = ['平台与账号需要成对填写；不需要的行请移除'];
      else if (seenAccounts.has(key)) problems[socialPath] = ['此平台与账号已经填写'];
      seenAccounts.add(key);
    }
    for (const purpose of row.primaryFor ?? []) {
      if (!(row.purposes ?? []).includes(purpose)) problems[path] = ['主要联系用途需要先选为联系用途'];
      if (primary.has(purpose)) problems[path] = [`${contactPurposeLabels[purpose]}已指名其他主要联系人`];
      primary.add(purpose);
    }
  }
  return problems;
}

/**
 * 修订号建议值：已取回的资料修订里最大的修订号 + 1，没有为 1。只省一次翻册，连续性由服务端判。取最大而不借
 * suggestedNextRevision：那份共用规则吃的是「每个对象一行最新修订」的目录列表，这里拿到的是整条修订链。
 */
export function suggestedProfileRevision(
  revisions: readonly LegalEntityProfileRevisionRecord[] | null,
  legalEntityId: string,
): number {
  const id = legalEntityId.trim();
  const latest = (revisions ?? [])
    .filter((row) => row.legalEntityId === id)
    .reduce((highest, row) => Math.max(highest, row.revision), 0);
  return latest + 1;
}

export type LegalEntityProfileStatusTone = 'good' | 'info' | 'warn' | 'bad' | 'muted';

export interface LegalEntityProfileStatus {
  label: string;
  tone: LegalEntityProfileStatusTone;
  description: string;
}

/**
 * 资料状态的词与一句说明，词照 company-subject-ux spec「统一用语」：可开票 / 资料不全 / 未登记资料 / 主体未生效 / 主体已停用。
 * 资料不全是明确的非成功——开立方据此拒开，这里不拿默认值补齐，说明句直接说后果。
 */
export function legalEntityProfileStatusOf(
  resolution: Pick<LegalEntityProfileResolutionResponseBody, 'outcome' | 'incompleteCause' | 'effectiveRevision'>,
): LegalEntityProfileStatus {
  if (resolution.outcome === 'RESOLVED') {
    if (resolution.effectiveRevision === undefined) {
      return { label: '状态未识别', tone: 'bad', description: '查询结果没有返回当前生效的资料版本。' };
    }
    return {
      label: '可开票',
      tone: 'good',
      description: '当前有生效资料且开票资料齐全。',
    };
  }
  if (resolution.outcome === 'PROFILE_INCOMPLETE') {
    if (resolution.incompleteCause === 'NO_EFFECTIVE_REVISION') {
      return { label: '未登记资料', tone: 'warn', description: '这个日期没有生效的资料。' };
    }
    if (resolution.incompleteCause === 'NO_INVOICING_DETAILS') {
      if (resolution.effectiveRevision === undefined) {
        return { label: '状态未识别', tone: 'bad', description: '资料不全的查询结果没有返回对应的生效版本。' };
      }
      return { label: '资料不全', tone: 'warn', description: '缺开票抬头，开票会被拒：生效的那一版没有开票抬头。' };
    }
    return { label: '状态未识别', tone: 'bad', description: '资料不全的查询结果没有返回可识别原因。' };
  }
  if (resolution.outcome === 'LEGAL_ENTITY_NOT_EFFECTIVE') {
    return { label: '主体未生效', tone: 'info', description: '公司主体在这个日期还没有生效。' };
  }
  if (resolution.outcome === 'LEGAL_ENTITY_DEACTIVATED') {
    return { label: '主体已停用', tone: 'muted', description: '公司主体已停用，不能再用于签约或开票。' };
  }
  if (resolution.outcome === 'LEGAL_ENTITY_NOT_REGISTERED') {
    return { label: '主体不在册', tone: 'bad', description: '当前租户没有登记这个公司主体。' };
  }
  return { label: '状态未识别', tone: 'bad', description: '读取结果包含无法识别的资料状态。' };
}

/** 资料层（税务登记号）的类型候选：目录里 layer 为 PROFILE 且国家 = 注册地址国家的类型，显目录译名；已停用的照列、括注。 */
export function profileTaxTypeOptions(
  types: readonly RegistrationNumberTypeRecord[],
  country: string,
): { value: string; label: string }[] {
  const code = country.trim();
  return types
    .filter((type) => type.layer === 'PROFILE' && type.countryCode === code)
    .map((type) => ({
      value: type.typeCode,
      label: `${type.typeName}${type.deactivatedAt !== undefined ? '（已停用）' : ''}`,
    }));
}

/**
 * 按时点解析要问的时刻：留空即「此刻」（取自调用方给的 now），填了就按操作者时区把墙钟换成 RFC 3339；换不出来交 null，
 * 由调用方拦着不发请求。
 */
export function resolutionInstantOf(wallTime: string, timeZone: string, now: Date): string | null {
  if (wallTime.trim() === '') return now.toISOString();
  return wallTimeToRfc3339(wallTime, timeZone);
}

export function representativeLabel(kind: string): string {
  return kind === 'LEGAL_REPRESENTATIVE' ? '法定代表人' : kind === 'RESPONSIBLE_PERSON' ? '登记负责人' : kind;
}

function addressPayload(country: string, lines: string): RegisteredAddressRecord | undefined {
  const value = {
    country: country.trim(),
    lines: lines
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean),
  };
  return value.country || value.lines.length ? value : undefined;
}

/** 没有地址显「—」（spec「统一用语」的空值规则）：空值不是结论句，不配一句话。 */
export function addressText(address?: RegisteredAddressRecord): string {
  return address ? [address.country, ...address.lines].filter(Boolean).join(' · ') : '—';
}

export function legalEntityContactCountOf(resolution: LegalEntityProfileResolutionResponseBody | undefined): number | undefined {
  if (!resolution || !resolution.effectiveRevision) return undefined;
  if (resolution.outcome === 'RESOLVED' ||
      (resolution.outcome === 'PROFILE_INCOMPLETE' && resolution.incompleteCause === 'NO_INVOICING_DETAILS')) {
    return resolution.effectiveRevision.contacts.length;
  }
  return undefined;
}

export function profileMaintenanceNotes(content: LegalEntityProfileContent): string[] {
  const notes: string[] = [];
  if (!content.representative) notes.push('法定代表人 / 登记负责人未配置');
  if (!content.contacts.some((row) => row.email || row.phone || row.socialAccounts?.length)) notes.push('未配置可联系的邮箱、电话或社交账号');
  const purposes = new Set(content.contacts.flatMap((row) => row.purposes ?? []));
  for (const purpose of purposes) {
    if (!content.contacts.some((row) => row.primaryFor?.includes(purpose)))
      notes.push(`${contactPurposeLabels[purpose] ?? purpose}主要联系人未指名`);
  }
  return notes;
}

/**
 * 表单打开时的草稿：基于一版已登记的内容（当前生效的那一版，或操作者另选的）预填内容格；来源单据、生效时间与版本号
 * 不沿用——那三格说的是「这一次更新」，不是资料内容。没有基线就是空白草稿。
 */
export function legalEntityProfileDraftBasedOn(
  legalEntityId: string,
  base: LegalEntityProfileContent | null,
): LegalEntityProfileDraft {
  const empty = emptyLegalEntityProfileDraft(legalEntityId);
  return base === null ? empty : { ...empty, ...profileDraftContentOf(base) };
}

/**
 * 草稿折成资料内容的形状，给保存前的差异预览与当前生效版比（compareLegalEntityProfiles）。经载荷折一遍而不直接读草稿：
 * 预览要说的是「将要送上去的」，去空白、空行不送、半空的联系人行丢掉，都与载荷一致；开票抬头缺席即这一版不带开票资料，
 * 与服务端对载荷的读法同一条。版本号、来源单据与生效时间不参与比较，照载荷带着只为凑齐形状。
 */
export function profileComparisonContentOf(draft: LegalEntityProfileDraft, timeZone: string): LegalEntityProfileContent {
  const item = legalEntityProfilePayloadOf(draft, timeZone).profiles[0];
  const content: LegalEntityProfileContent = {
    revision: item.revision ?? 0,
    basis: item.basis,
    effectiveFrom: item.effectiveFrom ?? '',
    registeredAddress: item.registeredAddress ?? { country: '', lines: [] },
    taxRegistrationNumbers: item.taxRegistrationNumbers ?? [],
    invoicingRegistered: item.invoiceTitle !== undefined,
    contacts: item.contacts ?? [],
  };
  if (item.invoiceTitle !== undefined) content.invoiceTitle = item.invoiceTitle;
  if (item.representative) content.representative = item.representative;
  if (item.officeAddress) content.officeAddress = item.officeAddress;
  if (item.correspondenceAddress) content.correspondenceAddress = item.correspondenceAddress;
  return content;
}

export function profileDraftContentOf(
  content: LegalEntityProfileContent,
): Pick<
  LegalEntityProfileDraft,
  | 'addressCountry'
  | 'addressLines'
  | 'invoiceTitle'
  | 'taxNumbers'
  | 'contacts'
  | 'representativeKind'
  | 'representativeName'
  | 'officeCountry'
  | 'officeLines'
  | 'correspondenceCountry'
  | 'correspondenceLines'
> {
  return {
    addressCountry: content.registeredAddress.country,
    addressLines: content.registeredAddress.lines.join('\n'),
    invoiceTitle: content.invoicingRegistered ? (content.invoiceTitle ?? '') : '',
    taxNumbers: content.taxRegistrationNumbers.map((row) => ({ ...row })),
    contacts: content.contacts.map((row) => ({
      ...row,
      email: row.email ?? '',
      phone: row.phone ?? '',
      purposes: [...(row.purposes ?? [])],
      primaryFor: [...(row.primaryFor ?? [])],
      socialAccounts: row.socialAccounts?.map((account) => ({ ...account })),
    })),
    representativeKind: content.representative?.kind ?? '',
    representativeName: content.representative?.name ?? '',
    officeCountry: content.officeAddress?.country ?? '',
    officeLines: content.officeAddress?.lines.join('\n') ?? '',
    correspondenceCountry: content.correspondenceAddress?.country ?? '',
    correspondenceLines: content.correspondenceAddress?.lines.join('\n') ?? '',
  };
}
