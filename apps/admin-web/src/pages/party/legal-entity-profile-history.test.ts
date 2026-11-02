import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { LegalEntityContactRecord, LegalEntityProfileRevisionRecord } from './api';
import {
  compareLegalEntityProfiles, orderedProfileRevisions, profileComparisonSummary,
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
