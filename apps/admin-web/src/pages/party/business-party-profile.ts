// 往来单位资料（ADR-0369）的表单纯逻辑：草稿 → 载荷、必填未填、本地编码层问题、修订号建议与「基于此刻有效的一版」预填。
// 资料各项的形状与法人资料一样，所以草稿就是法人资料那一份（LegalEntityProfileDraft）外加其他语言名称，字段组件也共用
// （LegalEntityProfileFields）；这里只做两处不同：主体是往来单位、不带法定代表人，注册地址可缺。全部是纯函数，node:test 钉着。
//
// 不裁领域门：往来单位在不在册、是不是我方公司、注册地址国家对不对得上、号合不合目录，一律送上去由服务端答。

import type {
  BusinessPartyProfileRevisionListResponseBody,
  BusinessPartyProfileRevisionRecord,
  LegalEntityProfileContent,
  LocalizedPartyNameRecord,
} from './api';
import {
  emptyLegalEntityProfileDraft,
  legalEntityProfileLocalProblems,
  legalEntityProfileMissingFields,
  legalEntityProfilePayloadOf,
  profileDraftContentOf,
  type LegalEntityProfileDraft,
  type LegalEntityProfileItem,
  type ProfileMissingField,
} from './legal-entity-profile';

/** 法人资料草稿的那一份（legalEntityId 一格不用，留空）外加往来单位编码与其他语言名称。 */
export interface BusinessPartyProfileDraft extends LegalEntityProfileDraft {
  partyId: string;
  names: LocalizedPartyNameRecord[];
}

export type BusinessPartyProfileItem = Omit<LegalEntityProfileItem, 'legalEntityId' | 'representative'> & {
  partyId: string;
  names?: LocalizedPartyNameRecord[];
};

export interface BusinessPartyProfilePayload {
  profiles: BusinessPartyProfileItem[];
}

export function emptyBusinessPartyProfileDraft(partyId: string): BusinessPartyProfileDraft {
  return { ...emptyLegalEntityProfileDraft(''), partyId, names: [] };
}

/**
 * 草稿 → 载荷：内容各格照法人资料的编码（去空白、空行不送、可缺即缺席），再换主体格、去掉法定代表人，带上其他语言名称（整行空着
 * 不送，填了一半照送，由服务端点名）。
 */
export function businessPartyProfilePayloadOf(draft: BusinessPartyProfileDraft, timeZone: string): BusinessPartyProfilePayload {
  const { legalEntityId: _subject, representative: _representative, ...content } = legalEntityProfilePayloadOf(draft, timeZone).profiles[0];
  const item: BusinessPartyProfileItem = { partyId: draft.partyId.trim(), ...content };
  const names = draft.names
    .map((row) => ({ language: row.language.trim(), name: row.name.trim() }))
    .filter((row) => row.language !== '' || row.name !== '');
  if (names.length > 0) item.names = names;
  return { profiles: [item] };
}

/** 必填未填：法人资料那一份去掉注册地址两格——往来单位资料的注册地址可缺。 */
export function businessPartyProfileMissingFields(draft: BusinessPartyProfileDraft): ProfileMissingField[] {
  return legalEntityProfileMissingFields(draft).filter((field) => !field.path.startsWith('profiles[0].registeredAddress'));
}

/** 本地编码层问题：法人资料那几条，外加其他语言名称成对、语言不重复。 */
export function businessPartyProfileLocalProblems(draft: BusinessPartyProfileDraft, timeZone: string): Record<string, string[]> {
  const problems = legalEntityProfileLocalProblems(draft, timeZone);
  // 注册地址在法人资料里是必填（缺项清单管它），这里可缺，就与办公、通信地址同一条：国家与地址行要么都填、要么都空。
  if (Boolean(draft.addressCountry.trim()) !== Boolean(draft.addressLines.trim())) {
    problems['profiles[0].registeredAddress'] = ['国家 / 地区与地址需要一起填写'];
  }
  const seen = new Set<string>();
  draft.names.forEach((row, index) => {
    const language = row.language.trim();
    const name = row.name.trim();
    const path = `profiles[0].names[${index}]`;
    if (!language || !name) {
      if (language || name) problems[path] = ['语言与名称需要成对填写；不需要的行请移除'];
      return;
    }
    if (seen.has(language.toLowerCase())) problems[path] = ['此语言已经填写'];
    seen.add(language.toLowerCase());
  });
  return problems;
}

/** 修订号建议值：已取回的修订里最大的 + 1，没有为 1。连续性由服务端判。 */
export function suggestedBusinessPartyProfileRevision(revisions: readonly BusinessPartyProfileRevisionRecord[] | null): number {
  return (revisions ?? []).reduce((highest, row) => Math.max(highest, row.revision), 0) + 1;
}

/** 此刻有效的那一笔：服务端答的修订号对上的那一行；没有一笔生效、或答复里找不到为 null。 */
export function effectiveBusinessPartyProfile(
  body: BusinessPartyProfileRevisionListResponseBody,
): BusinessPartyProfileRevisionRecord | null {
  if (body.effectiveRevision === undefined) return null;
  return body.revisions.find((row) => row.revision === body.effectiveRevision) ?? null;
}

/** 往来单位的修订折成法人资料内容的形状，好借法人资料的预填折算：注册地址缺席折成空的。 */
function asProfileContent(record: BusinessPartyProfileRevisionRecord): LegalEntityProfileContent {
  return {
    revision: record.revision,
    basis: record.basis,
    effectiveFrom: record.effectiveFrom,
    registeredAddress: record.registeredAddress ?? { country: '', lines: [] },
    taxRegistrationNumbers: record.taxRegistrationNumbers,
    invoicingRegistered: record.invoicingRegistered,
    invoiceTitle: record.invoiceTitle,
    invoiceTitleLanguage: record.invoiceTitleLanguage,
    invoiceTitles: record.invoiceTitles,
    contacts: record.contacts,
    officeAddress: record.officeAddress,
    correspondenceAddress: record.correspondenceAddress,
    bankAccounts: record.bankAccounts,
  };
}

/**
 * 表单打开时的草稿：基于此刻有效的那一版预填内容格；来源单据、生效时间与版本号不沿用——那三格说的是「这一次更新」。没有基线
 * 就是空白草稿。
 */
export function businessPartyProfileDraftBasedOn(
  partyId: string,
  base: BusinessPartyProfileRevisionRecord | null,
): BusinessPartyProfileDraft {
  const empty = emptyBusinessPartyProfileDraft(partyId);
  if (base === null) return empty;
  return { ...empty, ...profileDraftContentOf(asProfileContent(base)), names: (base.names ?? []).map((row) => ({ ...row })) };
}

/** 将要送上去的内容与基线比有没有改动：只比内容各格，版本号、来源单据与生效时间不算。 */
export function businessPartyProfileChanged(
  base: BusinessPartyProfileDraft,
  draft: BusinessPartyProfileDraft,
  timeZone: string,
): boolean {
  const contentOf = (value: BusinessPartyProfileDraft) => {
    const { revision: _revision, basis: _basis, effectiveFrom: _from, ...content } = businessPartyProfilePayloadOf(value, timeZone).profiles[0];
    return JSON.stringify(content);
  };
  return contentOf(base) !== contentOf(draft);
}
