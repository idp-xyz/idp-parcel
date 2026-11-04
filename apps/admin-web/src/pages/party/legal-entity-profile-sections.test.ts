import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { emptyLegalEntityProfileDraft, legalEntityProfileMissingFields } from './legal-entity-profile';
import type { ProfileComparison, ProfileFieldChange } from './legal-entity-profile-history';
import {
  draftWithNewRow,
  profileSectionEditingOf,
  profileSectionBasisOf,
  profileSectionOwnsPath,
  profileSections,
  splitMissingBySection,
} from './legal-entity-profile-sections';

// Covers: 分区认领路径看前缀——同前缀的子路径、带下标的行都算；别区的不算；前缀按整段认，「invoiceTitle」不顺带认走别的格。
test('分区按路径前缀认领格', () => {
  const invoicing = profileSections.invoicing;
  equal(profileSectionOwnsPath(invoicing, 'profiles[0].invoiceTitle'), true);
  equal(profileSectionOwnsPath(invoicing, 'profiles[0].invoiceTitleLanguage'), true);
  equal(profileSectionOwnsPath(profileSections.contacts, 'profiles[0].contactsExtra'), false);
  equal(profileSectionOwnsPath(invoicing, 'profiles[0].invoiceTitles[1]'), true);
  equal(profileSectionOwnsPath(invoicing, 'profiles[0].taxRegistrationNumbers[0].typeCode'), true);
  equal(profileSectionOwnsPath(invoicing, 'profiles[0].contacts[0]'), false);
  equal(profileSectionOwnsPath(profileSections.contacts, 'profiles[0].contacts[2].socialAccounts[0]'), true);
  equal(profileSectionOwnsPath(profileSections.representativeAndAddresses, 'profiles[0].registeredAddress.lines'), true);
});

// Covers: 改联系人时注册地址空着——那是别区的缺项，分到 outside；联系人姓名与来源单据、生效时间分到 inside。
test('必填缺项按区分成本区的与别区的', () => {
  const draft = {
    ...emptyLegalEntityProfileDraft('SYN-LE-01'),
    contacts: [{ name: '', email: '', phone: '' }],
  };
  const { inside, outside } = splitMissingBySection(profileSections.contacts, legalEntityProfileMissingFields(draft));
  deepEqual(
    inside.map((field) => field.path),
    ['profiles[0].contacts[0]', 'profiles[0].basis', 'profiles[0].effectiveFrom'],
  );
  deepEqual(
    outside.map((field) => field.path),
    ['profiles[0].registeredAddress', 'profiles[0].registeredAddress.lines'],
  );
});

// Covers: 从「添加」打开逐行的区末尾多一空行、焦点落在新行；不逐行的区原样交回、不给焦点。
test('从「添加」打开先加一空行', () => {
  const base = { ...emptyLegalEntityProfileDraft('SYN-LE-01'), contacts: [{ name: 'SYN 甲', email: '', phone: '' }] };
  const contacts = draftWithNewRow(profileSections.contacts, base);
  equal(contacts.draft.contacts.length, 2);
  equal(contacts.draft.contacts[1].name, '');
  equal(contacts.focusPath, 'profiles[0].contacts[1]');
  equal(base.contacts.length, 1);
  const accounts = draftWithNewRow(profileSections.bankAccounts, base);
  equal(accounts.draft.bankAccounts.length, 1);
  equal(accounts.focusPath, 'profiles[0].bankAccounts[0].holderName');
  const addresses = draftWithNewRow(profileSections.representativeAndAddresses, base);
  equal(addresses.draft, base);
  equal(addresses.focusPath, null);
});

function change(over: Partial<ProfileFieldChange> & Pick<ProfileFieldChange, 'group' | 'field' | 'kind'>): ProfileFieldChange {
  return { id: `${over.group}-${over.field}-${Math.random()}`, subject: '', detail: '', before: '', after: '', ...over };
}
function comparison(fields: ProfileFieldChange[]): ProfileComparison {
  return { fields, contactMatchingLimited: false };
}

// Covers: 自动依据只数本区的改动、按主体归并（一个联系人改三格是一处「修改」）、只点三处其余并成「等 N 处」；
// 收款账户的主体名去掉「收款账户 · 」；什么都没改也写一句。
test('来源单据按本区改动自动写一句', () => {
  const contacts = profileSections.contacts;
  const mixed = comparison([
    change({ group: '联系人', field: '联系人资料', kind: 'added', subject: '张三', contactKey: 'contact-1' }),
    change({ group: '联系人', field: '邮箱', kind: 'modified', subject: '李四', contactKey: 'contact-2' }),
    change({ group: '联系人', field: '电话', kind: 'added', subject: '李四', contactKey: 'contact-2' }),
    change({ group: '联系人', field: '职务', kind: 'removed', subject: '李四', contactKey: 'contact-2' }),
    change({ group: '联系人', field: '姓名', kind: 'unchanged', subject: '王五', contactKey: 'contact-3' }),
    change({ group: '地址信息', field: '注册地址', kind: 'modified' }),
  ]);
  equal(profileSectionBasisOf(contacts, mixed), '更新联系人：新增 张三、修改 李四');
  equal(profileSectionBasisOf(profileSections.representativeAndAddresses, mixed), '更新代表人与地址：修改 注册地址');
  equal(profileSectionBasisOf(profileSections.invoicing, mixed), '更新税务与开票：内容未变，重新登记一版');
  const many = comparison(
    ['甲', '乙', '丙', '丁', '戊'].map((name, index) =>
      change({ group: '联系人', field: '联系人资料', kind: 'removed', subject: name, contactKey: `contact-${index}` }),
    ),
  );
  equal(profileSectionBasisOf(contacts, many), '更新联系人：移除 甲、移除 乙、移除 丙 等 5 处');
  const account = comparison([
    change({ group: '收款账户', field: '收款账户 · 人民币（CNY） 6222', kind: 'added', subject: '收款账户 · 人民币（CNY） 6222' }),
  ]);
  equal(profileSectionBasisOf(profileSections.bankAccounts, account), '更新收款账户：新增 人民币（CNY） 6222');
});

test('读出卡的请求折成页面持有的编辑态，钉住公司', () => {
  deepEqual(profileSectionEditingOf('SYN-LE-01', { section: 'contacts', addRow: true }), { legalEntityId: 'SYN-LE-01', section: 'contacts', addRow: true });
  deepEqual(profileSectionEditingOf('SYN-LE-01', { section: 'invoicing' }), { legalEntityId: 'SYN-LE-01', section: 'invoicing', addRow: false });
});
