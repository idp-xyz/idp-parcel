import type {
  ContactPurpose,
  LegalEntityBankAccountRecord,
  LegalEntityContactRecord,
  LegalEntityProfileContent,
  LegalEntityProfileResolutionResponseBody,
  LegalEntityProfileRevisionRecord,
  RegisteredAddressRecord,
} from './api';
import { countryLabel, currencyLabel } from '../refdata-names';
import type { RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { contactPurposeLabels, languageTagLabel, representativeLabel } from './legal-entity-profile';

export type ProfileChangeKind = 'added' | 'modified' | 'removed' | 'unchanged';
export interface ProfileFieldChange {
  id: string;
  group: string;
  subject: string;
  detail: string;
  field: string;
  before: string;
  after: string;
  kind: ProfileChangeKind;
  contactKey?: string;
  social?: boolean;
}
export interface ProfileComparison {
  fields: ProfileFieldChange[];
  contactMatchingLimited: boolean;
}

export function orderedProfileRevisions(rows: readonly LegalEntityProfileRevisionRecord[]) {
  return [...rows].sort((a, b) =>
    (Date.parse(b.registeredAt) || 0) - (Date.parse(a.registeredAt) || 0) || b.revision - a.revision,
  );
}

export function selectedProfileRevision(
  rows: readonly LegalEntityProfileRevisionRecord[],
  selected: number | null,
  current: number | undefined,
) {
  return rows.find((row) => row.revision === selected)
    ?? rows.find((row) => row.revision === current)
    ?? orderedProfileRevisions(rows)[0];
}

export function profileRevisionState(
  row: LegalEntityProfileRevisionRecord,
  resolution?: LegalEntityProfileResolutionResponseBody,
): { label: string; tone: 'current' | 'future' | 'muted' } {
  if (!resolution || !['RESOLVED', 'PROFILE_INCOMPLETE'].includes(resolution.outcome)) {
    return { label: '状态未判读', tone: 'muted' };
  }
  if (resolution.effectiveRevision?.revision === row.revision) {
    return { label: '当前生效', tone: 'current' };
  }
  if (Date.parse(row.effectiveFrom) > Date.parse(resolution.at)) {
    return { label: '未来生效', tone: 'future' };
  }
  return { label: resolution.effectiveRevision ? '历史版本' : '已登记', tone: 'muted' };
}

/**
 * 对比表里的地址文本：地址行在前用「 · 」连，国家 / 地区经基础数据译名（译名表没读回时显码）并在行末——与资料读出的
 * ProfileReadoutAddress 同一版式（票 company-subject-ux/08 第 5 条起国家不再另起一行）。没有地址交空串，调用方按「没有」比。
 */
export function profileAddressText(address?: RegisteredAddressRecord): string {
  if (!address) return '';
  const lines = address.lines.filter((line) => line.trim() !== '');
  return [...lines, address.country ? countryLabel(address.country) : ''].filter(Boolean).join(' · ');
}

const values = (items: readonly string[] = []) => [...items].sort();
const purposeText = (items: readonly ContactPurpose[] = []) =>
  values(items).map((value) => contactPurposeLabels[value as ContactPurpose] ?? value).join('、');

function socialGroups(contact: LegalEntityContactRecord) {
  const groups = new Map<string, { platform: string; accounts: string[] }>();
  for (const row of contact.socialAccounts ?? []) {
    const key = row.platform.trim().toLowerCase();
    const group = groups.get(key) ?? { platform: row.platform.trim(), accounts: [] };
    group.accounts.push(row.account.trim());
    groups.set(key, group);
  }
  for (const group of groups.values()) group.accounts.sort();
  return groups;
}

/**
 * 收款账户以（币种，账号）认同一个账户——服务端也拿这一对判重复（ADR-0359 决定三），所以换了币种或账号就是移除一个、
 * 新增一个，其余格变了才是「修改」。账号去首尾空白后原样比，IBAN 带不带空格算两个账号，与服务端同一口径。
 */
function bankAccountKey(account: LegalEntityBankAccountRecord) {
  return JSON.stringify([account.currency.trim(), account.accountNumber.trim()]);
}
/** 对比格里一个账户的文本：币种与账号已在项目名里，这里只列其余格，空着的不列。 */
function bankAccountText(account: LegalEntityBankAccountRecord) {
  return [
    account.label, account.holderName, account.bankName,
    account.swiftCode ? `SWIFT / BIC ${account.swiftCode}` : '', account.bankAddress, account.note,
  ].filter(Boolean).join(' · ');
}
/** 项目名「收款账户 · 人民币（CNY） SYN-…」：币种经基础数据译名（译名表没读回时显码），账号原样。 */
export function bankAccountSubject(account: LegalEntityBankAccountRecord) {
  return `收款账户 · ${currencyLabel(account.currency.trim())} ${account.accountNumber.trim()}`;
}

// 接口没有联系人标识：只有其他资料完全一致时，才将社交账号作为同一联系人的字段变更。
function baseContactKey(contact: LegalEntityContactRecord) {
  return JSON.stringify([
    contact.name, contact.department ?? '', contact.position ?? '',
    contact.email ?? '', contact.phone ?? '', values(contact.purposes), values(contact.primaryFor),
  ]);
}
function fullContactKey(contact: LegalEntityContactRecord) {
  return JSON.stringify([baseContactKey(contact), [...socialGroups(contact)].sort(([a], [b]) => a.localeCompare(b))
    .map(([platform, group]) => [platform, group.accounts])]);
}
function contactText(contact: LegalEntityContactRecord) {
  return [contact.name, contact.department, contact.position, contact.email, contact.phone,
    purposeText(contact.purposes), contact.primaryFor?.length ? `主要联系人：${purposeText(contact.primaryFor)}` : '',
    ...[...socialGroups(contact).values()].map((group) => `${group.platform}：${group.accounts.join('；')}`),
  ].filter(Boolean).join(' · ');
}

/**
 * `typeNameOf` 给了就把监管登记号的类型码译成目录译名再比（按各自的注册地址国家查）：比的是显出来的文本，所以换了国家、
 * 同一个码译出不同名时也算一处变化——国家本来就变了，这一行跟着变不是误报。不给按码比。
 */
export function compareLegalEntityProfiles(
  before: LegalEntityProfileContent,
  after: LegalEntityProfileContent,
  typeNameOf?: RegistrationNumberTypeNameOf,
): ProfileComparison {
  const fields: ProfileFieldChange[] = [];
  let contactMatchingLimited = false;
  const add = (
    id: string, group: string, field: string, left = '', right = '',
    subject = '', detail = '', contactKey?: string, social = false,
  ) => {
    const kind: ProfileChangeKind = left === right ? 'unchanged' : !left ? 'added' : !right ? 'removed' : 'modified';
    fields.push({ id, group, field, before: left, after: right, kind, subject, detail, contactKey, social });
  };
  const address = profileAddressText;
  const representative = (content: LegalEntityProfileContent) => content.representative
    ? `${representativeLabel(content.representative.kind)}：${content.representative.name}` : '';
  const tax = (content: LegalEntityProfileContent) => values(content.taxRegistrationNumbers
    .map((row) => `${typeNameOf?.(content.registeredAddress.country, row.typeCode, 'PROFILE') ?? row.typeCode} ${row.number}`)).join('；');
  add('representative', '代表人信息', '代表人 / 负责人', representative(before), representative(after));
  add('registered-address', '地址信息', '注册地址', address(before.registeredAddress), address(after.registeredAddress));
  add('office-address', '地址信息', '办公地址', address(before.officeAddress), address(after.officeAddress));
  add('correspondence-address', '地址信息', '通信地址', address(before.correspondenceAddress), address(after.correspondenceAddress));
  add('tax-numbers', '税务与开票', '监管登记号（税务 / 海关）', tax(before), tax(after));
  add('invoicing', '税务与开票', '开票资料', before.invoicingRegistered ? '已登记' : '未登记', after.invoicingRegistered ? '已登记' : '未登记');
  add('invoice-title', '税务与开票', '开票抬头', before.invoiceTitle ?? '', after.invoiceTitle ?? '');
  add('invoice-title-language', '税务与开票', '主抬头语言',
    languageTagLabel(before.invoiceTitleLanguage ?? ''), languageTagLabel(after.invoiceTitleLanguage ?? ''));
  // 其他语言的抬头一种语言一行，语言是键（服务端也按语言判重复、不分大小写）：换了语言是一移除一新增，抬头变了是修改。
  const oldTitles = new Map((before.invoiceTitles ?? []).map((row) => [row.language.trim().toLowerCase(), row]));
  const newTitles = new Map((after.invoiceTitles ?? []).map((row) => [row.language.trim().toLowerCase(), row]));
  let titleSequence = 0;
  for (const key of new Set([...oldTitles.keys(), ...newTitles.keys()])) {
    const left = oldTitles.get(key);
    const right = newTitles.get(key);
    add(`invoice-titles-${++titleSequence}`, '税务与开票', `其他语言的抬头 · ${languageTagLabel((right ?? left)!.language)}`,
      left?.title.trim() ?? '', right?.title.trim() ?? '');
  }

  // 账户按旧版次序列、新加的排在后面：读的人先对旧版，再看多了什么。
  const oldAccounts = new Map((before.bankAccounts ?? []).map((account) => [bankAccountKey(account), account]));
  const newAccounts = new Map((after.bankAccounts ?? []).map((account) => [bankAccountKey(account), account]));
  let accountSequence = 0;
  for (const key of new Set([...oldAccounts.keys(), ...newAccounts.keys()])) {
    const left = oldAccounts.get(key);
    const right = newAccounts.get(key);
    add(`bank-account-${++accountSequence}`, '收款账户', bankAccountSubject((right ?? left)!),
      left ? bankAccountText(left) : '', right ? bankAccountText(right) : '');
  }

  const oldGroups = new Map<string, LegalEntityContactRecord[]>();
  const newGroups = new Map<string, LegalEntityContactRecord[]>();
  for (const [contacts, groups] of [[before.contacts, oldGroups], [after.contacts, newGroups]] as const) {
    for (const contact of contacts) {
      const key = baseContactKey(contact);
      groups.set(key, [...(groups.get(key) ?? []), contact]);
    }
  }
  let sequence = 0;
  const pair = (left: LegalEntityContactRecord, right: LegalEntityContactRecord) => {
    const key = `contact-${++sequence}`;
    const detail = [right.department, right.position].filter(Boolean).join(' · ');
    const contactField = (field: string, a: string, b: string, social = false) =>
      add(`${key}-${fields.length}`, '联系人', field, a, b, right.name, detail, key, social);
    contactField('姓名', left.name, right.name);
    contactField('部门', left.department ?? '', right.department ?? '');
    contactField('职务', left.position ?? '', right.position ?? '');
    contactField('邮箱', left.email ?? '', right.email ?? '');
    contactField('电话', left.phone ?? '', right.phone ?? '');
    contactField('联系用途', purposeText(left.purposes), purposeText(right.purposes));
    contactField('主要联系人用途', purposeText(left.primaryFor), purposeText(right.primaryFor));
    const a = socialGroups(left);
    const b = socialGroups(right);
    const platforms = new Set([...a.keys(), ...b.keys()]);
    for (const platform of platforms) {
      contactField((b.get(platform) ?? a.get(platform))!.platform,
        a.get(platform)?.accounts.join('；') ?? '', b.get(platform)?.accounts.join('；') ?? '', true);
    }
    if (!platforms.size) contactField('社交账号', '', '', true);
  };
  const removed: LegalEntityContactRecord[] = [];
  const added: LegalEntityContactRecord[] = [];
  for (const key of new Set([...oldGroups.keys(), ...newGroups.keys()])) {
    const left = [...(oldGroups.get(key) ?? [])];
    const right = [...(newGroups.get(key) ?? [])];
    // 先消除完全相同项，避免同名联系人中不变的记录挤占唯一可比较项。
    for (let i = left.length - 1; i >= 0; i--) {
      const match = right.findIndex((row) => fullContactKey(row) === fullContactKey(left[i]));
      if (match < 0) continue;
      pair(left[i], right[match]);
      left.splice(i, 1);
      right.splice(match, 1);
    }
    if (left.length === 1 && right.length === 1) pair(left[0], right[0]);
    else {
      if (left.length && right.length) contactMatchingLimited = true;
      removed.push(...left);
      added.push(...right);
    }
  }
  if (removed.length && added.length) contactMatchingLimited = true;
  for (const [contacts, removal] of [[removed, true], [added, false]] as const) {
    for (const contact of contacts) {
      const key = `contact-${++sequence}`;
      add(key, '联系人', '联系人资料', removal ? contactText(contact) : '', removal ? '' : contactText(contact),
        contact.name, [contact.department, contact.position].filter(Boolean).join(' · '), key);
    }
  }
  return { fields, contactMatchingLimited };
}

export function profileComparisonSummary(comparison?: ProfileComparison) {
  if (!comparison) return '首次登记';
  const changed = comparison.fields.filter((row) => row.kind !== 'unchanged');
  if (!changed.length) return '资料内容无变化';
  if (changed.every((row) => row.social)) {
    const contacts = new Set(changed.map((row) => row.contactKey)).size;
    return `${contacts} 位联系人${changed.every((row) => row.kind === 'added') ? '新增' : '更新'}社交账号`;
  }
  if (changed.length === 1) {
    const action = changed[0].kind === 'added' ? '新增' : changed[0].kind === 'removed' ? '移除' : '修改';
    return `${action}${changed[0].field}`;
  }
  return `${changed.length} 个字段发生变化`;
}
