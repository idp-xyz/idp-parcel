// 法人详情「法人资料」区的纯逻辑（票 legal-entity-profile/04 第 2 项；ADR-0145 决定三、五、六）：登记表单的草稿 → 载荷、
// 本地编码层问题、修订号建议，以及当前有效、修订历史两处共用的内容转写。全部是纯函数，node:test 钉着；组件
// LegalEntityProfileSection.tsx 只负责摆。
//
// **不算校验结论**（票面「不做」）：法人在不在册、地址国家与身份上的注册国家对不对得上、税号合不合目录、修订连不连续，
// 一律送上去让服务端答。载荷镜像受控 CLI `register-legal-entity-profiles` 的一项、不带租户格——理由同法人登记表单
// （legal-entity-form.ts 文件头）。

import type {
  LegalEntityContactRecord,
  LegalEntityProfileContent,
  LegalEntityProfileRevisionRecord,
  RegisteredAddressRecord,
  TaxRegistrationNumberRecord,
} from './api';
import { wallTimeToRfc3339 } from '../moment';
import { revisionOf } from './registration-form';
import type { RevisionTimelineItem } from './revision-timeline';

export interface TaxNumberDraft {
  typeCode: string;
  number: string;
}

export interface ContactDraft {
  name: string;
  email: string;
  phone: string;
}

export interface LegalEntityProfileDraft {
  /** 抽屉里那个法人，不是输入格：资料挂在哪个法人上由抽屉定。 */
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
  };
}

/**
 * 草稿 → 载荷。各格去首尾空白后原样带；可缺的键留空即缺席、不送空串（Intake 对它们「给了就得立得住」，空串会被当成
 * 给了而答形状错；缺席才轮到用例按 ADR-0145 答「未受理」并点名）。注册地址国家与各行全空才算没给，填了一部分照带。
 */
export function legalEntityProfilePayloadOf(draft: LegalEntityProfileDraft, timeZone: string): LegalEntityProfilePayload {
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
  const contacts = draft.contacts
    .map((row) => ({ name: row.name.trim(), email: row.email.trim(), phone: row.phone.trim() }))
    .filter((row) => row.name !== '' || row.email !== '' || row.phone !== '')
    .map((row) => {
      const contact: LegalEntityContactRecord = { name: row.name };
      if (row.email !== '') contact.email = row.email;
      if (row.phone !== '') contact.phone = row.phone;
      return contact;
    });
  if (contacts.length > 0) item.contacts = contacts;
  return { profiles: [item] };
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。三种：修订号编不进正整数、生效时刻换不成 RFC 3339、
 * 税务登记号有一行只填了类型或只填了号（半行编不成 Intake 要的一项）。其余一律是服务端的话。
 */
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

export interface ProfileContentLine {
  label: string;
  value: string;
}

/**
 * 一笔资料的内容照答复原样转写：国家码、地址各行、税号类型码都不译不补。开票资料看 invoicingRegistered 那个显式布尔，
 * 为假就写「未登开票资料」——按时点解析会据此答资料不全，这里不拿抬头缺席去推。
 */
export function profileContentLines(content: LegalEntityProfileContent): ProfileContentLine[] {
  const address = [content.registeredAddress.country, ...content.registeredAddress.lines].filter((part) => part !== '');
  return [
    { label: '注册地址', value: address.length === 0 ? '—' : address.join(' · ') },
    {
      label: '税务登记号',
      value:
        content.taxRegistrationNumbers.length === 0
          ? '—'
          : content.taxRegistrationNumbers.map((entry) => `${entry.typeCode} ${entry.number}`).join('；'),
    },
    { label: '开票抬头', value: content.invoicingRegistered ? (content.invoiceTitle ?? '') : '未登开票资料' },
    {
      label: '联系人',
      value: content.contacts.length === 0 ? '—' : content.contacts.map(contactText).join('；'),
    },
  ];
}

function contactText(contact: LegalEntityContactRecord): string {
  const reach = [contact.email, contact.phone].filter((part): part is string => part !== undefined && part !== '');
  return reach.length === 0 ? contact.name : `${contact.name}（${reach.join('，')}）`;
}

/**
 * 修订历史的时间线条目：按修订号从新到旧。标题带生效时点——资料按生效时点取代前一修订，未来生效的那笔也在链上，
 * 看标题就分得出哪笔还没到。
 */
export function profileRevisionTimeline(
  revisions: readonly LegalEntityProfileRevisionRecord[],
  format: (iso: string) => string,
): RevisionTimelineItem[] {
  return [...revisions]
    .sort((left, right) => right.revision - left.revision)
    .map((row) => ({
      id: `r${row.revision}`,
      title: `r${row.revision} · 生效自 ${format(row.effectiveFrom)}`,
      description: [
        ...profileContentLines(row).map((line) => `${line.label} ${line.value}`),
        `依据 ${row.basis}`,
      ].join(' · '),
      timestamp: format(row.registeredAt),
      variant: 'default' as const,
    }));
}

export function profileRevisionHistoryNote(count: number): string {
  return count === 0
    ? '这个法人还没有登记过资料（法人可以先登记、后补资料，ADR-0145 决定六）。'
    : `共 ${count} 笔资料修订，从新到旧；新修订自其生效时点起取代前一修订，登记过的不被覆盖。`;
}

/**
 * 按时点解析要问的时刻：留空即「此刻」（取自调用方给的 now），填了就按操作者时区把墙钟换成 RFC 3339；换不出来交 null，
 * 由调用方拦着不发请求。
 */
export function resolutionInstantOf(wallTime: string, timeZone: string, now: Date): string | null {
  if (wallTime.trim() === '') return now.toISOString();
  return wallTimeToRfc3339(wallTime, timeZone);
}
