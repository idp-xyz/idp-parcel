import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { LegalEntityProfileRevisionRecord } from './api';
import {
  emptyLegalEntityProfileDraft,
  legalEntityProfileLocalProblems,
  legalEntityProfilePayloadOf,
  legalEntityProfileStatusOf,
  legalEntityContactCountOf,
  profileDraftContentOf,
  profileMaintenanceNotes,
  profileContentLines,
  profileRevisionTimeline,
  resolutionInstantOf,
  suggestedProfileRevision,
  type LegalEntityProfileDraft,
} from './legal-entity-profile';

test('社交账号带入再保存保全且不共享可变资料，部分输入与重复组合提示问题', () => {
  const source = revision({ contacts: [{ name: 'SYN 联系人', socialAccounts: [{ platform: '微信', account: 'SYN-WX-01' }, { platform: 'Telegram', account: 'SYN-TG-01' }] }] });
  const content = profileDraftContentOf(source);
  const payload = legalEntityProfilePayloadOf(draft(content), 'UTC').profiles[0];
  deepEqual(payload.contacts, source.contacts);
  content.contacts[0].socialAccounts![0].account = 'SYN-CHANGED';
  equal(source.contacts[0].socialAccounts![0].account, 'SYN-WX-01');
  const broken = draft({ contacts: [{ name: 'SYN', email: '', phone: '', socialAccounts: [{ platform: '微信', account: '' }, { platform: 'Telegram', account: 'SYN-01' }, { platform: ' telegram ', account: ' SYN-01 ' }] }] });
  const problems = legalEntityProfileLocalProblems(broken, 'UTC');
  equal(problems['profiles[0].contacts[0].socialAccounts[0]']?.length, 1);
  equal(problems['profiles[0].contacts[0].socialAccounts[2]']?.length, 1);
  equal(profileMaintenanceNotes(source).some(note => note.includes('未配置可联系')), false);
});

test('联系人计数区分真实零人、没有可读资料与开票不全', () => {
  const response = { outcome: 'RESOLVED', legalEntityId: 'SYN-LE-01', at: '2026-10-30T08:00:00Z', effectiveRevision: revision({ contacts: [] }) };
  equal(legalEntityContactCountOf(response), 0);
  const withContacts = { ...response, effectiveRevision: revision({ contacts: [{ name: 'SYN-A' }, { name: 'SYN-B' }] }) };
  equal(legalEntityContactCountOf(withContacts), 2);
  equal(legalEntityContactCountOf({ ...withContacts, outcome: 'PROFILE_INCOMPLETE', incompleteCause: 'NO_INVOICING_DETAILS' }), 2);
  equal(legalEntityContactCountOf(undefined), undefined);
  equal(legalEntityContactCountOf({ ...response, effectiveRevision: undefined }), undefined);
  for (const outcome of ['LEGAL_ENTITY_DEACTIVATED', 'LEGAL_ENTITY_NOT_EFFECTIVE', 'LEGAL_ENTITY_NOT_REGISTERED', 'UNKNOWN']) {
    equal(legalEntityContactCountOf({ ...withContacts, outcome }), undefined);
  }
  equal(legalEntityContactCountOf({ ...withContacts, outcome: 'PROFILE_INCOMPLETE', incompleteCause: 'NO_EFFECTIVE_REVISION' }), undefined);
});

test('扩展资料带入和再保存保全内容，但不沿用旧登记决定', () => {
  const source = revision({
    representative: { kind: 'LEGAL_REPRESENTATIVE', name: 'SYN 代表人' },
    officeAddress: { country: 'SG', lines: ['SYN 办公地址'] },
    correspondenceAddress: { country: 'CN', lines: ['SYN 通信地址'] },
    invoicingRegistered: true,
    invoiceTitle: 'SYN 抬头',
    contacts: [
      {
        name: 'SYN 财务联系人',
        department: 'SYN 财务部',
        position: 'SYN 经理',
        purposes: ['FINANCE', 'BUSINESS'],
        primaryFor: ['FINANCE'],
        email: 'finance@example.invalid',
      },
    ],
  });
  const content = profileDraftContentOf(source);
  equal('basis' in content, false);
  equal('effectiveFrom' in content, false);
  equal('revision' in content, false);
  const payload = legalEntityProfilePayloadOf(draft(content), 'UTC').profiles[0];
  deepEqual(payload.contacts, source.contacts);
  deepEqual(payload.representative, source.representative);
  deepEqual(payload.officeAddress, source.officeAddress);
  deepEqual(payload.correspondenceAddress, source.correspondenceAddress);
  content.contacts[0].primaryFor!.push('BUSINESS');
  equal(source.contacts[0].primaryFor!.length, 1);
  deepEqual(profileMaintenanceNotes(source), ['商务主要联系人未指名']);
});

test('扩展可选格不产生默认值；半填和主要联系人冲突明确报告', () => {
  const blank = legalEntityProfilePayloadOf(draft({}), 'UTC').profiles[0];
  equal('representative' in blank, false);
  equal('officeAddress' in blank, false);
  equal('correspondenceAddress' in blank, false);
  const problems = legalEntityProfileLocalProblems(
    draft({
      representativeName: 'SYN 代表人',
      officeCountry: 'CN',
      contacts: [
        { name: 'SYN-A', email: '', phone: '', purposes: ['FINANCE'], primaryFor: ['FINANCE'] },
        { name: 'SYN-B', email: '', phone: '', purposes: ['FINANCE'], primaryFor: ['FINANCE'] },
        { name: '', email: 'finance@example.invalid', phone: '' },
      ],
    }),
    'UTC',
  );
  deepEqual(problems['profiles[0].representative'], ['身份类型与姓名需要成对填写']);
  deepEqual(problems['profiles[0].officeAddress'], ['国家 / 地区与地址需要一起填写']);
  deepEqual(problems['profiles[0].contacts[1]'], ['财务已指名其他主要联系人']);
  deepEqual(problems['profiles[0].contacts[2]'], ['请填写联系人姓名']);
  const old = profileDraftContentOf(revision({}));
  equal(old.representativeKind, '');
  equal(old.officeCountry, '');
});

// 本文件钉法人资料区的纯逻辑（票 legal-entity-profile/04 第 2 项）：表单只做编码层的事——修订号编整数、墙钟换 RFC 3339、
// 可缺键缺席而不是空串；法人在不在册、地址国家对不对得上身份、税号合不合目录一律送给服务端答。内容照答复原样转写。

function draft(over: Partial<LegalEntityProfileDraft>): LegalEntityProfileDraft {
  return {
    ...emptyLegalEntityProfileDraft('SYN-LE-01'),
    revision: '2',
    basis: ' SYN-PROFILE-BASIS-02 ',
    effectiveFrom: '2027-01-01T08:00',
    ...over,
  };
}

// Covers: 各格齐 → profiles 一项；修订号整数、生效时刻按时区换 UTC（可在未来）；地址逐行、空行不送；税号与联系人按行成项，
// 联系人的邮箱与电话留空即键缺席；载荷不带租户格。
test('草稿组成载荷：一项、各格去空白、可缺键按填没填', () => {
  deepEqual(
    legalEntityProfilePayloadOf(
      draft({
        addressCountry: ' CN ',
        addressLines: ' SYN 地址一行 \n\n SYN 地址二行 ',
        taxNumbers: [
          { typeCode: ' SYN-VAT ', number: ' 123 ' },
          { typeCode: '', number: ' ' },
        ],
        invoiceTitle: ' SYN 开票抬头 ',
        contacts: [
          { name: ' SYN 联系人 ', email: ' syn@example.invalid ', phone: '' },
          { name: '', email: '', phone: ' ' },
        ],
      }),
      'Asia/Shanghai',
    ),
    {
      profiles: [
        {
          legalEntityId: 'SYN-LE-01',
          revision: 2,
          basis: 'SYN-PROFILE-BASIS-02',
          effectiveFrom: '2027-01-01T00:00:00Z',
          registeredAddress: { country: 'CN', lines: ['SYN 地址一行', 'SYN 地址二行'] },
          taxRegistrationNumbers: [{ typeCode: 'SYN-VAT', number: '123' }],
          invoiceTitle: 'SYN 开票抬头',
          contacts: [{ name: 'SYN 联系人', email: 'syn@example.invalid' }],
        },
      ],
    },
  );
});

// Covers: 可缺的四样全空 → 键缺席（不送空串：Intake 对它们「给了就得立得住」；开票抬头空串服务端照样拒）；
// 地址只填了国家也照带，立不立得住由服务端判。
test('全空的可缺格缺席，填了一部分的地址照带', () => {
  const blank = legalEntityProfilePayloadOf(draft({ invoiceTitle: '  ' }), 'UTC').profiles[0];
  equal('registeredAddress' in blank, false);
  equal('taxRegistrationNumbers' in blank, false);
  equal('invoiceTitle' in blank, false);
  equal('contacts' in blank, false);
  deepEqual(legalEntityProfilePayloadOf(draft({ addressCountry: 'SG' }), 'UTC').profiles[0].registeredAddress, {
    country: 'SG',
    lines: [],
  });
});

// Covers: 本地只报编码层三件——修订号、生效时刻、半填的税号行（按行号）；其余不在本地判。
test('本地只报编码层问题', () => {
  deepEqual(legalEntityProfileLocalProblems(draft({}), 'UTC'), {});
  deepEqual(
    legalEntityProfileLocalProblems(
      draft({
        revision: '0',
        effectiveFrom: 'someday',
        addressCountry: 'NOT-A-COUNTRY',
        taxNumbers: [
          { typeCode: 'SYN-VAT', number: '' },
          { typeCode: 'SYN-GST', number: '9' },
        ],
      }),
      'UTC',
    ),
    {
      'profiles[0].revision': ['修订号要填正整数'],
      'profiles[0].effectiveFrom': ['生效时刻要是可解析的日期时间'],
      'profiles[0].taxRegistrationNumbers': ['第 1 行：类型与号要成对填'],
    },
  );
});

function revision(over: Partial<LegalEntityProfileRevisionRecord>): LegalEntityProfileRevisionRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    legalEntityId: 'SYN-LE-01',
    revision: 1,
    basis: 'SYN-PROFILE-BASIS-01',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAddress: { country: 'CN', lines: ['SYN 地址'] },
    taxRegistrationNumbers: [],
    invoicingRegistered: false,
    contacts: [],
    registeredAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 修订号建议取已登资料修订的最新一笔 + 1；读不到（null）或没有为 1。
test('修订号建议', () => {
  equal(suggestedProfileRevision([revision({}), revision({ revision: 3 })], 'SYN-LE-01'), 4);
  equal(suggestedProfileRevision([], 'SYN-LE-01'), 1);
  equal(suggestedProfileRevision(null, 'SYN-LE-01'), 1);
});

// Covers: 内容照答复原样转写；缺少的资料明示未配置；开票资料看显式布尔；联系人带上给了的联络方式。
test('资料内容转写', () => {
  deepEqual(profileContentLines(revision({})), [
    { label: '注册地址', value: 'CN · SYN 地址' },
    { label: '税务登记号', value: '未配置' },
    { label: '开票抬头', value: '未登开票资料' },
    { label: '联系人', value: '未配置' },
  ]);
  deepEqual(
    profileContentLines(
      revision({
        taxRegistrationNumbers: [
          { typeCode: 'SYN-VAT', number: '1' },
          { typeCode: 'SYN-GST', number: '2' },
        ],
        invoicingRegistered: true,
        invoiceTitle: 'SYN 抬头',
        contacts: [{ name: 'SYN 甲', email: 'a@example.invalid', phone: '100' }, { name: 'SYN 乙' }],
      }),
    ).slice(1),
    [
      { label: '税务登记号', value: 'SYN-VAT 1；SYN-GST 2' },
      { label: '开票抬头', value: 'SYN 抬头' },
      { label: '联系人', value: 'SYN 甲（a@example.invalid，100）；SYN 乙' },
    ],
  );
  deepEqual(profileContentLines(revision({ registeredAddress: { country: '', lines: [] } })).slice(0, 1), [
    { label: '注册地址', value: '未配置' },
  ]);
});

test('法人资料状态按当前解析结果区分未配置、资料不全与可开票', () => {
  deepEqual(legalEntityProfileStatusOf({ outcome: 'RESOLVED', effectiveRevision: revision({}) }), {
    label: '开票资料可用',
    tone: 'good',
    description: '当前时点有生效资料，且已登记开票资料。',
  });
  equal(
    legalEntityProfileStatusOf({ outcome: 'PROFILE_INCOMPLETE', incompleteCause: 'NO_EFFECTIVE_REVISION' }).label,
    '未配置',
  );
  equal(
    legalEntityProfileStatusOf({
      outcome: 'PROFILE_INCOMPLETE',
      incompleteCause: 'NO_INVOICING_DETAILS',
      effectiveRevision: revision({}),
    }).label,
    '资料不全',
  );
  equal(legalEntityProfileStatusOf({ outcome: 'LEGAL_ENTITY_DEACTIVATED' }).label, '主体已停用');
  equal(legalEntityProfileStatusOf({ outcome: 'PROFILE_INCOMPLETE', incompleteCause: 'UNKNOWN' }).label, '状态未识别');
  equal(
    legalEntityProfileStatusOf({ outcome: 'PROFILE_INCOMPLETE', incompleteCause: 'NO_INVOICING_DETAILS' }).label,
    '状态未识别',
  );
  equal(legalEntityProfileStatusOf({ outcome: 'RESOLVED' }).label, '状态未识别');
});

// Covers: 历史从新到旧；标题短写「第 N 版」（轴名由「资料版本」签承担，spec 公司主体 UX「统一用语」）并带生效时点，
// 未来生效的那笔看标题就分得出。
test('资料修订历史从新到旧、标题带生效时点', () => {
  const items = profileRevisionTimeline(
    [revision({}), revision({ revision: 2, effectiveFrom: '2027-01-01T00:00:00Z' })],
    (iso) => `<${iso}>`,
  );
  deepEqual(
    items.map((item) => item.title),
    ['第 2 版 · 生效自 <2027-01-01T00:00:00Z>', '第 1 版 · 生效自 <2026-01-01T00:00:00Z>'],
  );
  equal(items[1].timestamp, '<2026-01-01T00:00:00Z>');
});

// Covers: 按时点解析要问的时刻——留空即此刻（取调用方给的 now），填了按时区换 UTC，换不出来交 null 不发请求。
test('解析时刻：留空即此刻，填了换 UTC', () => {
  const now = new Date('2026-09-24T12:00:00Z');
  equal(resolutionInstantOf('', 'UTC', now), '2026-09-24T12:00:00.000Z');
  equal(resolutionInstantOf('2027-01-01T08:00', 'Asia/Shanghai', now), '2027-01-01T00:00:00Z');
  equal(resolutionInstantOf('later', 'UTC', now), null);
});
