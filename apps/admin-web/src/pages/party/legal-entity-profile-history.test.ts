import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { LegalEntityContactRecord, LegalEntityProfileRevisionRecord } from './api';
import { contactPurposeLabels } from './legal-entity-profile';
import {
  compareLegalEntityProfiles, orderedProfileRevisions, profileAddressText, profileComparisonSummary,
  profileRevisionState, selectedProfileRevision,
} from './legal-entity-profile-history';

const contact = (overrides: Partial<LegalEntityContactRecord> = {}): LegalEntityContactRecord => ({
  name: 'SYN 联系人', department: '财务部', purposes: ['FINANCE'], primaryFor: ['FINANCE'], ...overrides,
});
const revision = (overrides: Partial<LegalEntityProfileRevisionRecord> = {}): LegalEntityProfileRevisionRecord => ({
  tenantId: 'SYN-TENANT', legalEntityId: 'SYN-LE', revision: 1, basis: 'SYN-BASIS',
  effectiveFrom: '2026-01-01T00:00:00Z', registeredAt: '2026-01-01T00:00:00Z',
  registeredAddress: { country: 'CN', lines: ['SYN 地址'] }, taxRegistrationNumbers: [],
  invoicingRegistered: false, contacts: [], ...overrides,
});
const changed = (a: LegalEntityProfileRevisionRecord, b: LegalEntityProfileRevisionRecord) =>
  compareLegalEntityProfiles(a, b).fields.filter((row) => row.kind !== 'unchanged');

test('社交账号按平台形成差异，不重复铺开未变的联系人资料', () => {
  const contacts = [contact(), contact({ name: 'SYN 运营' })];
  const a = revision({ contacts });
  const b = revision({ contacts: contacts.map((row) => ({ ...row, socialAccounts: [
    { platform: '微信', account: 'SYN-WX' }, { platform: '企业微信', account: 'SYN-WORK' },
  ] })) });
  equal(changed(a, b).length, 4);
  equal(changed(a, b).every((row) => row.kind === 'added' && row.social), true);
  equal(profileComparisonSummary(compareLegalEntityProfiles(a, b)), '2 位联系人新增社交账号');
});

test('联系人、用途、税号和社交账号顺序不制造内容变化', () => {
  const first = contact({ purposes: ['FINANCE', 'BUSINESS'], socialAccounts: [
    { platform: 'Telegram', account: 'B' }, { platform: 'telegram', account: 'A' },
  ] });
  const second = contact({ name: 'SYN B' });
  const a = revision({ contacts: [first, second], taxRegistrationNumbers: [
    { typeCode: 'A', number: '1' }, { typeCode: 'B', number: '2' },
  ] });
  const b = revision({ contacts: [second, { ...first, purposes: ['BUSINESS', 'FINANCE'],
    socialAccounts: [{ platform: 'TELEGRAM', account: 'A' }, { platform: 'Telegram', account: 'B' }] }],
    taxRegistrationNumbers: [...a.taxRegistrationNumbers].reverse(),
  });
  equal(changed(a, b).length, 0);
});

test('同名不同部门不串行；同平台多账号完整保留', () => {
  const a = revision({ contacts: [contact(), contact({ department: '运营部' })] });
  const b = revision({ contacts: [contact({ department: '运营部', socialAccounts: [
    { platform: '微信', account: 'SYN-2' }, { platform: '微信', account: 'SYN-1' },
  ] }), contact()] });
  const rows = changed(a, b);
  equal(rows.length, 1);
  equal(rows[0].detail, '运营部');
  equal(rows[0].after, 'SYN-1；SYN-2');
});

test('没有稳定标识的资料修改按移除新增提示，不按数组下标猜测', () => {
  const a = revision({ contacts: [contact()] });
  const b = revision({ contacts: [contact({ name: 'SYN 新姓名', primaryFor: [] })] });
  const result = compareLegalEntityProfiles(a, b);
  equal(result.contactMatchingLimited, true);
  deepEqual(result.fields.filter((row) => row.kind !== 'unchanged').map((row) => row.kind), ['removed', 'added']);
  equal(result.fields.find((row) => row.kind === 'removed')?.before.includes('主要联系人：财务'), true);
  const ambiguous = compareLegalEntityProfiles(revision({ contacts: [contact(), contact()] }),
    revision({ contacts: [contact({ socialAccounts: [{ platform: '微信', account: 'A' }] }),
      contact({ socialAccounts: [{ platform: '微信', account: 'B' }] })] }));
  equal(ambiguous.contactMatchingLimited, true);
});

test('账号区分大小写，新增移除方向正确', () => {
  const a = revision({ contacts: [contact({ socialAccounts: [{ platform: '微信', account: 'ABC' }] })] });
  const b = revision({ contacts: [contact({ socialAccounts: [{ platform: '微信', account: 'abc' }] }), contact({ name: 'SYN B' })] });
  deepEqual(changed(a, b).map((row) => row.kind), ['modified', 'added']);
  equal(changed(b, a).some((row) => row.kind === 'removed'), true);
});

test('当前生效只认后端时点解析，不认最大修订号', () => {
  const current = revision({ revision: 2 });
  const future = revision({ revision: 3, effectiveFrom: '2027-01-01T00:00:00Z' });
  const resolution = { outcome: 'RESOLVED', legalEntityId: 'SYN-LE', at: '2026-10-30T00:00:00Z', effectiveRevision: current };
  equal(profileRevisionState(current, resolution).label, '当前生效');
  equal(profileRevisionState(future, resolution).label, '未来生效');
  equal(profileRevisionState(current).label, '状态未判读');
  equal(profileRevisionState(current, { ...resolution, outcome: 'PROFILE_INCOMPLETE' }).label, '当前生效');
});

test('默认当前版，保留明确选择，缺失时回退；登记时间排序不修改原数组', () => {
  const rows = [revision(), revision({ revision: 3, registeredAt: '2026-10-30T00:00:00Z' }), revision({ revision: 2 })];
  equal(selectedProfileRevision(rows, null, 2)?.revision, 2);
  equal(selectedProfileRevision(rows, 1, 2)?.revision, 1);
  equal(selectedProfileRevision(rows, 99, undefined)?.revision, 3);
  equal(selectedProfileRevision([], null, undefined), undefined);
  deepEqual(orderedProfileRevisions(rows).map((row) => row.revision), [3, 2, 1]);
  equal(rows[0].revision, 1);
});

test('地址开票独立比较，版本时间和依据元数据不混入资料差异', () => {
  const a = revision();
  const b = revision({ revision: 2, basis: 'NEW', effectiveFrom: '2026-10-30T00:00:00Z',
    officeAddress: { country: 'CN', lines: ['SYN 办公室'] }, invoiceTitle: 'SYN 抬头' });
  deepEqual(changed(a, b).map((row) => row.field), ['办公地址', '开票抬头']);
  equal(profileComparisonSummary(compareLegalEntityProfiles(a, { ...a, revision: 99, basis: 'NEW' })), '资料内容无变化');
  equal(profileComparisonSummary(), '首次登记');
});

// Covers: 对比表里的地址行在前、国家并在行末（译名表没读回时显码），不再把国家码当前缀；空行不占位；没有地址是空串，
// 两版都没有不算变化、一版有一版没有按新增 / 移除。
test('地址文本行在前国家在行末，没有地址是空串', () => {
  equal(profileAddressText({ country: 'CN', lines: ['合成市', '', '合成路 1 号'] }), '合成市 · 合成路 1 号 · CN');
  equal(profileAddressText({ country: '', lines: ['合成市'] }), '合成市');
  equal(profileAddressText(undefined), '');
  const a = revision();
  const b = revision({ officeAddress: { country: 'CN', lines: ['SYN 办公室'] } });
  deepEqual(changed(a, b).map((row) => [row.field, row.kind, row.after]), [['办公地址', 'added', 'SYN 办公室 · CN']]);
  equal(changed(a, revision()).length, 0);
});

test('完整资料快照数据保留主要联系人用途', () => {
  const row = revision({ contacts: [contact({ primaryFor: ['FINANCE'] })] });
  equal(row.contacts[0].primaryFor?.map((purpose) => contactPurposeLabels[purpose]).join('、'), '财务');
});

// Covers（票 company-subject-ux/06 第 2 条）：收款账户以（币种，账号）认同一个账户——其余格变了是「修改」，换了币种或账号是
// 一移除一新增；次序不制造变化；项目名带币种与账号，格里只列其余格。
test('收款账户按币种与账号对应：修改、移除、新增各成一行，次序不算变化', () => {
  const cny = { label: '人民币对公', holderName: 'SYN 户名', bankName: 'SYN 银行', accountNumber: 'SYN-CNY-01', swiftCode: 'SYNBCNBJ', currency: 'CNY' };
  const usd = { holderName: 'SYN Holder', bankName: 'SYN Bank', accountNumber: 'SYN-USD-01', currency: 'USD' };
  const a = revision({ bankAccounts: [cny, usd] });
  deepEqual(changed(a, revision({ bankAccounts: [usd, cny] })), []);
  const b = revision({ bankAccounts: [
    { ...cny, bankAddress: 'SYN 新地址' },
    { ...usd, accountNumber: 'SYN-USD-02' },
  ] });
  deepEqual(
    changed(a, b).map((row) => [row.group, row.field, row.kind, row.before, row.after]),
    [
      ['收款账户', '收款账户 · CNY SYN-CNY-01', 'modified', '人民币对公 · SYN 户名 · SYN 银行 · SWIFT / BIC SYNBCNBJ', '人民币对公 · SYN 户名 · SYN 银行 · SWIFT / BIC SYNBCNBJ · SYN 新地址'],
      ['收款账户', '收款账户 · USD SYN-USD-01', 'removed', 'SYN Holder · SYN Bank', ''],
      ['收款账户', '收款账户 · USD SYN-USD-02', 'added', '', 'SYN Holder · SYN Bank'],
    ],
  );
  equal(profileComparisonSummary(compareLegalEntityProfiles(a, revision({ bankAccounts: [cny] }))), '移除收款账户 · USD SYN-USD-01');
  deepEqual(changed(revision({}), revision({ bankAccounts: [] })), []);
});

// Covers（票 company-subject-ux/06 第 3 条）：主抬头语言一行、其他语言的抬头一种语言一行（语言是键、不分大小写）：
// 抬头变了是修改、换了语言是一移除一新增；语言经常用表译名，译不出原样。
test('多语言抬头按语言对应：主抬头语言与各语言的抬头各成一行', () => {
  const a = revision({ invoicingRegistered: true, invoiceTitle: 'SYN', invoiceTitleLanguage: 'zh-Hans', invoiceTitles: [
    { language: 'en', title: 'SYN Title' }, { language: 'ja', title: 'SYN JA' },
  ] });
  deepEqual(changed(a, revision({ ...a, invoiceTitles: [{ language: 'JA', title: 'SYN JA' }, { language: 'en', title: 'SYN Title' }] })), []);
  const b = revision({ ...a, invoiceTitleLanguage: 'zh-Hant', invoiceTitles: [
    { language: 'en', title: 'SYN Title v2' }, { language: 'fr-CA', title: 'SYN FR' },
  ] });
  deepEqual(
    changed(a, b).map((row) => [row.field, row.kind, row.before, row.after]),
    [
      ['主抬头语言', 'modified', '简体中文（zh-Hans）', '繁体中文（zh-Hant）'],
      ['其他语言的抬头 · 英语（en）', 'modified', 'SYN Title', 'SYN Title v2'],
      ['其他语言的抬头 · 日语（ja）', 'removed', 'SYN JA', ''],
      ['其他语言的抬头 · fr-CA', 'added', '', 'SYN FR'],
    ],
  );
  deepEqual(changed(revision({}), revision({ invoiceTitles: [] })), []);
});
