// 往来单位资料（ADR-0369）的表单纯逻辑：草稿 → 载荷、必填未填、本地编码层问题、修订号建议与「基于此刻有效的一版」预填。
// 资料各项的形状与法人资料一样，所以草稿就是法人资料那一份（LegalEntityProfileDraft）外加其他语言名称，字段组件也共用
// （LegalEntityProfileFields）；这里只做两处不同：主体是往来单位、不带法定代表人，注册地址可缺。全部是纯函数，node:test 钉着。
//
// 不裁领域门：往来单位在不在册、是不是我方公司、注册地址国家对不对得上、号合不合目录，一律送上去由服务端答。

import type { ApiResult } from '../catalogue-api';
import type {
  BusinessPartyProfileRevisionListResponseBody,
  BusinessPartyProfileRevisionRecord,
  LegalEntityProfileContent,
  LocalizedPartyNameRecord,
} from './api';
import type { RegistrationNumberTypeNameOf } from './legal-entity-identity';
import {
  emptyLegalEntityProfileDraft,
  languageTagLabel,
  legalEntityProfileLocalProblems,
  legalEntityProfileMissingFields,
  legalEntityProfilePayloadOf,
  profileComparisonContentOf,
  profileDraftContentOf,
  type LegalEntityProfileDraft,
  type LegalEntityProfileItem,
  type ProfileMissingField,
} from './legal-entity-profile';
import {
  compareLegalEntityProfiles,
  type ProfileChangeKind,
  type ProfileComparison,
  type ProfileFieldChange,
} from './legal-entity-profile-history';

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

/**
 * 抬头名称下那一行：此刻有效资料里的其他语言名称。有英文取英文——对外单据与海外往来最常用；没有就取登记的第一个；没有为 null。
 */
export function otherLanguageNameOf(record: Pick<BusinessPartyProfileRevisionRecord, 'names'> | null): string | null {
  const names = (record?.names ?? []).filter((row) => row.name.trim() !== '');
  if (names.length === 0) return null;
  const english = names.find((row) => /^en(-|$)/i.test(row.language.trim()));
  return (english ?? names[0]).name;
}

/**
 * 客户缺联系人提示的判据：此刻持有客户角色，此刻有效的资料里却一位联系人都没有（没有资料也算）。只是提示——下单与对账要找得到
 * 人；不拦任何操作，也不替租户定几位才够。
 */
export function customerLacksContacts(
  roles: readonly string[],
  record: Pick<BusinessPartyProfileRevisionRecord, 'contacts'> | null,
): boolean {
  return roles.includes('CUSTOMER') && (record?.contacts ?? []).length === 0;
}

/**
 * 头部事实「资料」那一格。读到了却没有生效版时分开说「尚无生效版本」（登过、都在未来生效）与「未登记」；还在读是「…」，读不到
 * 是「读不到」——不拿「未登记」冒充，那会让人以为对方真没登资料。
 */
export function businessPartyProfileVersionFact(
  answer: ApiResult<BusinessPartyProfileRevisionListResponseBody> | null,
  partyId: string,
): string {
  if (answer === null) return '…';
  if (answer.kind !== 'outcome') return '读不到';
  if (answer.body.partyId !== partyId) return '…';
  const effective = effectiveBusinessPartyProfile(answer.body);
  if (effective) return `第 ${effective.revision} 版`;
  return answer.body.revisions.length > 0 ? '尚无生效版本' : '未登记';
}

/**
 * 读出几处共用的资料状态：还在读或读不到只给一句灰字；读到了却没有生效的一版是空状态（登过、都在未来生效的另说一句）；
 * 有生效版就交出那一版。别家的答复（换行时上一家的还没换掉）当作还在读。对象页的读出与工作台右栏各签都从这里判。
 */
export type BusinessPartyProfileState =
  | { kind: 'note'; text: string }
  | { kind: 'empty'; scheduled: boolean; body: BusinessPartyProfileRevisionListResponseBody }
  | { kind: 'present'; record: BusinessPartyProfileRevisionRecord; body: BusinessPartyProfileRevisionListResponseBody };

export function businessPartyProfileStateOf(
  answer: ApiResult<BusinessPartyProfileRevisionListResponseBody> | null,
  partyId: string,
): BusinessPartyProfileState {
  if (answer === null) return { kind: 'note', text: '正在读取资料…' };
  if (answer.kind !== 'outcome') return { kind: 'note', text: '资料暂时读不到，稍后刷新再看。' };
  if (answer.body.partyId !== partyId) return { kind: 'note', text: '正在读取资料…' };
  const record = effectiveBusinessPartyProfile(answer.body);
  if (record) return { kind: 'present', record, body: answer.body };
  return { kind: 'empty', scheduled: answer.body.revisions.length > 0, body: answer.body };
}

/** 资料空着时那一块的话：一句标题，下面几行说空在哪、下一步去哪。 */
export interface ProfileEmptyNote {
  title: string;
  lines: string[];
}

/** 客户有资料却没有联系人时的那句提示（判据 customerLacksContacts）。 */
export const customerWithoutContactsLine = '这家单位是客户，资料里还没有联系人；建议登记一位，下单和对账时找得到人。';

/**
 * 登过停用的往来单位资料空着时说的那一句：登记册不再收它新的一版资料（identity-write-entries.ts 的 identityTakesRevisions 为假），
 * 「在『更新资料』里登记」对它是假话，入口也不摆。说「已登记停用」而不说「已停用」：停用时点还没到的同样不收。
 */
export const profileClosedLine = '这家单位已登记停用，不再收新的资料。';

/**
 * 「概要」里资料空着：没登过与登过却都在未来生效分开说；是客户再补一句建议。`takesRevisions` 为假（登过停用）时去处换成
 * profileClosedLine，建议也不提——登不了的事不建议。
 */
export function profileSummaryEmptyNoteOf(scheduled: boolean, roles: readonly string[], takesRevisions: boolean): ProfileEmptyNote {
  const title = scheduled ? '还没有生效的资料' : '还没有登记资料';
  if (!takesRevisions) return { title, lines: scheduled ? ['已登记的版本都在未来生效。', profileClosedLine] : [profileClosedLine] };
  return {
    title,
    lines: [
      scheduled ? '已登记的版本都在未来生效。' : '联系人、地址、税号与收款账户都在「更新资料」里登记。',
      ...(customerLacksContacts(roles, null) ? ['这家单位是客户，建议登记一位联系人，下单和对账时找得到人。'] : []),
    ],
  };
}

/**
 * 「联系人」里一位都没有：资料在而没有联系人、登过却没有生效的资料、没登过资料各一个标题；下面一句说去处，登过停用的
 * （`takesRevisions` 为假）换成 profileClosedLine。
 */
export function profileContactsEmptyNoteOf(
  state: Exclude<BusinessPartyProfileState, { kind: 'note' }>,
  takesRevisions: boolean,
): ProfileEmptyNote {
  return {
    title: state.kind === 'present' ? '还没有登记联系人' : state.scheduled ? '还没有生效的资料' : '还没有登记资料',
    lines: [takesRevisions ? '联系人与地址、税号一起在「更新资料」里登记。' : profileClosedLine],
  };
}

/** 「资料版本」里一版都没有；登过停用的不再说「登记后」。 */
export function profileVersionsEmptyNoteOf(takesRevisions: boolean): ProfileEmptyNote {
  return { title: '还没有登记资料', lines: [takesRevisions ? '登记后每一版都列在这里，旧版本不会被覆盖。' : profileClosedLine] };
}

/** 往来单位的修订折成法人资料内容的形状，好借法人资料的预填折算与保存前的比较：注册地址缺席折成空的。 */
export function businessPartyProfileContentOf(record: BusinessPartyProfileRevisionRecord): LegalEntityProfileContent {
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
  return { ...empty, ...profileDraftContentOf(businessPartyProfileContentOf(base)), names: (base.names ?? []).map((row) => ({ ...row })) };
}

/**
 * 其他语言名称的改动，一种语言一行：语言是键（服务端也按语言判重复、不分大小写），换了语言是一移除一新增，名称变了才是修改。
 * 法人资料没有这一格，借来的那份比较（compareLegalEntityProfiles）不管它。
 */
function otherLanguageNameChanges(
  before: readonly LocalizedPartyNameRecord[],
  after: readonly LocalizedPartyNameRecord[],
): ProfileFieldChange[] {
  const keyed = (rows: readonly LocalizedPartyNameRecord[]) =>
    new Map(rows.filter((row) => row.language.trim() !== '').map((row) => [row.language.trim().toLowerCase(), row] as const));
  const left = keyed(before);
  const right = keyed(after);
  const changes: ProfileFieldChange[] = [];
  let sequence = 0;
  for (const key of new Set([...left.keys(), ...right.keys()])) {
    const was = left.get(key)?.name.trim() ?? '';
    const now = right.get(key)?.name.trim() ?? '';
    const kind: ProfileChangeKind = was === now ? 'unchanged' : !was ? 'added' : !now ? 'removed' : 'modified';
    changes.push({
      id: `other-language-name-${++sequence}`,
      group: '名称与地址',
      subject: '',
      detail: '',
      field: `其他语言名称 · ${languageTagLabel((right.get(key) ?? left.get(key))!.language.trim())}`,
      before: was,
      after: now,
      kind,
    });
  }
  return changes;
}

/**
 * 保存前核对用的比较（票 business-party-redesign/11）：将要送上去的内容与表单基于的那一版比。内容各格借法人资料那一份比较；
 * 其他语言名称另比、排在最前——读出里它也在「名称与地址」卡的头一格。比的是将要送的那一份（经载荷折过），去空白、空行不送
 * 都已经算进去，与 businessPartyProfileChanged 判「有没有改动」用的是同一份载荷。
 */
export function businessPartyProfileComparisonOf(
  base: BusinessPartyProfileRevisionRecord,
  draft: BusinessPartyProfileDraft,
  timeZone: string,
  typeNameOf?: RegistrationNumberTypeNameOf,
): ProfileComparison {
  const comparison = compareLegalEntityProfiles(businessPartyProfileContentOf(base), profileComparisonContentOf(draft, timeZone), typeNameOf);
  const sending = businessPartyProfilePayloadOf(draft, timeZone).profiles[0].names ?? [];
  return { ...comparison, fields: [...otherLanguageNameChanges(base.names ?? [], sending), ...comparison.fields] };
}

/**
 * 编辑态页头右端那枚标记上的话：在哪一版的基础上改。表单基于的那一版还没生效时（已登记的都在未来生效，预填用的是最新登记的
 * 那一版）说清它还没生效；一版都没有就是登记第一版。
 */
export function profileEditTagText(base: Pick<BusinessPartyProfileRevisionRecord, 'revision'> | null, scheduled: boolean): string {
  if (base === null) return '正在登记资料 · 第 1 版';
  return scheduled ? `正在更新资料 · 在尚未生效的第 ${base.revision} 版的基础上改` : `正在更新资料 · 在第 ${base.revision} 版的基础上改`;
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
