import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { LegalEntityProfileRevisionRecord, RegistrationNumberTypeRecord } from './api';
import { compareLegalEntityProfiles } from './legal-entity-profile-history';
import {
  addressLinesOf,
  addressText,
  bankAccountCurrencyOptions,
  emptyLegalEntityProfileDraft,
  invoiceTitleRowsOf,
  languageTagLabel,
  legalEntityProfileDraftBasedOn,
  legalEntityProfileLocalProblems,
  legalEntityProfileMissingFields,
  legalEntityProfilePayloadOf,
  legalEntityProfileStatusOf,
  legalEntityContactCountOf,
  problemsWithMissing,
  profileComparisonContentOf,
  profileDraftContentOf,
  profileMaintenanceNotes,
  profileTaxTypeOptions,
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
  deepEqual(profileMaintenanceNotes(source), ['未登记收款账户，对账单与发票上印不出收款账户', '商务主要联系人未指名']);
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

// Covers: 空地址显「—」（spec 空值规则），有地址国家与各行以「 · 」连。
test('地址转写：没有即「—」', () => {
  equal(addressText(undefined), '—');
  equal(addressText({ country: 'CN', lines: ['SYN 一行', 'SYN 二行'] }), 'CN · SYN 一行 · SYN 二行');
});

// Covers（票 company-subject-ux/12 第 1 条）：读出按录入的行显，空行与首尾空白剔掉、次序不动；国家不混进行里。
test('读出的地址行：照录入次序、剔空行与空白、不含国家', () => {
  deepEqual(addressLinesOf({ country: 'CN', lines: [' SYN 一行 ', '', '  ', 'SYN 二行'] }), ['SYN 一行', 'SYN 二行']);
  deepEqual(addressLinesOf({ country: 'CN', lines: [] }), []);
});

test('资料状态按当前查询结果区分未登记资料、资料不全与可开票，词照统一用语', () => {
  deepEqual(legalEntityProfileStatusOf({ outcome: 'RESOLVED', effectiveRevision: revision({}) }), {
    label: '可开票',
    tone: 'good',
    description: '当前有生效资料且开票资料齐全。',
  });
  equal(
    legalEntityProfileStatusOf({ outcome: 'PROFILE_INCOMPLETE', incompleteCause: 'NO_EFFECTIVE_REVISION' }).label,
    '未登记资料',
  );
  equal(legalEntityProfileStatusOf({ outcome: 'LEGAL_ENTITY_NOT_EFFECTIVE' }).label, '主体未生效');
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

// Covers（票 company-subject-ux/04 第 1 项）：基于一版预填只带内容格，来源单据 / 生效时间 / 版本号空着；没有基线即空白草稿。
test('表单草稿基于一版预填：内容格带入，这一次更新的三格空着', () => {
  const base = revision({ invoicingRegistered: true, invoiceTitle: 'SYN 抬头', contacts: [{ name: 'SYN 甲', email: 'a@example.invalid' }] });
  const based = legalEntityProfileDraftBasedOn('SYN-LE-01', base);
  equal(based.legalEntityId, 'SYN-LE-01');
  equal(based.invoiceTitle, 'SYN 抬头');
  equal(based.addressCountry, 'CN');
  deepEqual(based.contacts.map((row) => row.name), ['SYN 甲']);
  equal(based.basis, '');
  equal(based.effectiveFrom, '');
  equal(based.revision, '');
  deepEqual(legalEntityProfileDraftBasedOn('SYN-LE-01', null), emptyLegalEntityProfileDraft('SYN-LE-01'));
});

// Covers（第 2 项）：预填后不动直接折回去与基线无差异；改一格只列那一格；清空开票抬头在预览里是「移除 开票抬头」，
// 空白项移除旧值由预览直接显示；折算经载荷——去空白、半空联系人行丢掉。
test('草稿折成资料内容与基线比：不动无差异、改一格列一格、清空抬头是移除', () => {
  const base = revision({
    invoicingRegistered: true,
    invoiceTitle: 'SYN 抬头',
    taxRegistrationNumbers: [{ typeCode: 'SYN-CN-TAX', number: 'SYN-CN-TAX-0101' }],
    representative: { kind: 'LEGAL_REPRESENTATIVE', name: 'SYN 代表人' },
    officeAddress: { country: 'CN', lines: ['SYN 办公'] },
    contacts: [
      { name: 'SYN 甲', email: 'a@example.invalid', phone: '100', department: 'SYN 财务', purposes: ['FINANCE'], primaryFor: ['FINANCE'] },
      { name: 'SYN 乙', socialAccounts: [{ platform: '微信', account: 'SYN-WX' }] },
    ],
  });
  const untouched = legalEntityProfileDraftBasedOn('SYN-LE-01', base);
  const same = compareLegalEntityProfiles(base, profileComparisonContentOf(untouched, 'UTC'));
  deepEqual(same.fields.filter((row) => row.kind !== 'unchanged'), []);
  equal(same.contactMatchingLimited, false);

  const oneChange = compareLegalEntityProfiles(
    base,
    profileComparisonContentOf({ ...untouched, representativeName: ' SYN 新代表人 ' }, 'UTC'),
  );
  deepEqual(
    oneChange.fields.filter((row) => row.kind !== 'unchanged').map((row) => [row.field, row.kind, row.after]),
    [['代表人 / 负责人', 'modified', '法定代表人：SYN 新代表人']],
  );

  const cleared = profileComparisonContentOf({ ...untouched, invoiceTitle: '  ' }, 'UTC');
  equal(cleared.invoicingRegistered, false);
  equal('invoiceTitle' in cleared, false);
  deepEqual(
    compareLegalEntityProfiles(base, cleared).fields.filter((row) => row.kind !== 'unchanged').map((row) => [row.field, row.kind]),
    [['开票资料', 'modified'], ['开票抬头', 'removed']],
  );

  const blank = profileComparisonContentOf(emptyLegalEntityProfileDraft('SYN-LE-01'), 'UTC');
  deepEqual(blank, {
    revision: 0,
    basis: '',
    effectiveFrom: '',
    registeredAddress: { country: '', lines: [] },
    taxRegistrationNumbers: [],
    invoicingRegistered: false,
    contacts: [],
  });
  const halfContact = profileComparisonContentOf(
    { ...untouched, contacts: [...untouched.contacts, { name: '', email: '', phone: '' }] },
    'UTC',
  );
  equal(halfContact.contacts.length, 2);

  // 给了目录查法，监管登记号那一行按译名显（按各自的注册地址国家查）；查不到的码原样。
  // 查法收到的层是资料层（票 company-subject-ux/06 第 4 条）：同码两层都登时不拿身份层的名顶上来。
  const typeNameOf = (country: string, typeCode: string, layer?: string) => (country === 'CN' && typeCode === 'SYN-CN-TAX' && layer === 'PROFILE' ? 'SYN 税号' : undefined);
  const taxChanged = compareLegalEntityProfiles(
    base,
    profileComparisonContentOf({ ...untouched, taxNumbers: [{ typeCode: 'SYN-CN-TAX', number: 'SYN-CN-TAX-0102' }, { typeCode: 'SYN-OTHER', number: '9' }] }, 'UTC'),
    typeNameOf,
  );
  deepEqual(
    taxChanged.fields.filter((row) => row.kind !== 'unchanged').map((row) => [row.field, row.before, row.after]),
    [['监管登记号（税务 / 海关）', 'SYN 税号 SYN-CN-TAX-0101', 'SYN 税号 SYN-CN-TAX-0102；SYN-OTHER 9']],
  );
});

// Covers（第 4 项）：缺项按表单次序列出，路径与各格认领问题的路径一致；联系人整行空着也算；齐了为空；并进问题表只加一句。
test('必填未填按表单次序列出，并进问题表', () => {
  const blank = emptyLegalEntityProfileDraft('SYN-LE-01');
  deepEqual(legalEntityProfileMissingFields(blank), [
    { path: 'profiles[0].registeredAddress', label: '注册地址国家 / 地区' },
    { path: 'profiles[0].registeredAddress.lines', label: '注册地址' },
    { path: 'profiles[0].basis', label: '来源单据' },
    { path: 'profiles[0].effectiveFrom', label: '生效时间' },
  ]);
  deepEqual(
    legalEntityProfileMissingFields(draft({ addressCountry: 'CN', addressLines: 'SYN 地址', contacts: [{ name: 'SYN 甲', email: '', phone: '' }, { name: ' ', email: '', phone: '' }] })),
    [{ path: 'profiles[0].contacts[1]', label: '联系人 2 姓名' }],
  );
  deepEqual(legalEntityProfileMissingFields(draft({ addressCountry: 'CN', addressLines: 'SYN 地址' })), []);
  deepEqual(problemsWithMissing({ 'profiles[0].basis': ['服务端的话'] }, [{ path: 'profiles[0].basis', label: '来源单据' }, { path: 'profiles[0].effectiveFrom', label: '生效时间' }]), {
    'profiles[0].basis': ['服务端的话', '必填未填'],
    'profiles[0].effectiveFrom': ['必填未填'],
  });
});

// Covers（第 6 项）：候选只取资料层（PROFILE）且国家相符的类型，显目录译名、已停用括注；身份层与别国不收；没有即空。
test('监管登记号类型候选来自目录的资料层', () => {
  const type = (over: Partial<RegistrationNumberTypeRecord>): RegistrationNumberTypeRecord => ({
    tenantId: 'SYN-TENANT-01', countryCode: 'CN', typeCode: 'SYN-CN-TAX', revision: 1, typeName: 'SYN 税号',
    layer: 'PROFILE', formatPattern: '', basis: 'SYN', status: 'EFFECTIVE', effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z', ...over,
  });
  const types = [
    type({}),
    type({ typeCode: 'USCC', typeName: '统一社会信用代码', layer: 'IDENTITY' }),
    type({ countryCode: 'SG', typeCode: 'SYN-SG-TAX', typeName: 'SYN 新加坡税号' }),
    type({ countryCode: 'SG', typeCode: 'SYN-SG-TAX-OLD', typeName: 'SYN 旧税号', deactivatedAt: '2026-06-30T00:00:00Z' }),
  ];
  deepEqual(profileTaxTypeOptions(types, ' CN '), [{ value: 'SYN-CN-TAX', label: 'SYN 税号' }]);
  deepEqual(profileTaxTypeOptions(types, 'SG'), [
    { value: 'SYN-SG-TAX', label: 'SYN 新加坡税号' },
    { value: 'SYN-SG-TAX-OLD', label: 'SYN 旧税号（已停用）' },
  ]);
  deepEqual(profileTaxTypeOptions(types, 'HK'), []);
  deepEqual(profileTaxTypeOptions(types, ''), []);
});

// Covers: 按时点解析要问的时刻——留空即此刻（取调用方给的 now），填了按时区换 UTC，换不出来交 null 不发请求。
test('解析时刻：留空即此刻，填了换 UTC', () => {
  const now = new Date('2026-09-24T12:00:00Z');
  equal(resolutionInstantOf('', 'UTC', now), '2026-09-24T12:00:00.000Z');
  equal(resolutionInstantOf('2027-01-01T08:00', 'Asia/Shanghai', now), '2027-01-01T00:00:00Z');
  equal(resolutionInstantOf('later', 'UTC', now), null);
});

// Covers（票 company-subject-ux/06 第 2 条）：收款账户带入再保存原样保全（可缺格缺席、整行空着不送）；必填格空着进缺项清单、
// 按格点名；两行（币种，账号）相同先在本地提一句；有开票抬头没账户提「印不出收款账户」，没有抬头或有账户都不提。
test('收款账户：带入与再保存保全、缺项按格点名、重复先提、维护提示只在有抬头没账户时', () => {
  const source = revision({
    invoicingRegistered: true,
    invoiceTitle: 'SYN 抬头',
    bankAccounts: [
      { label: '人民币对公', holderName: 'SYN 户名', bankName: 'SYN 银行', accountNumber: 'SYN-CNY-01', swiftCode: 'SYNBCNBJ', bankAddress: 'SYN 地址', currency: 'CNY', note: 'SYN 备注' },
      { holderName: 'SYN Holder', bankName: 'SYN Bank', accountNumber: 'SYN-USD-01', currency: 'USD' },
    ],
  });
  const content = profileDraftContentOf(source);
  deepEqual(content.bankAccounts[1], { label: '', holderName: 'SYN Holder', bankName: 'SYN Bank', accountNumber: 'SYN-USD-01', swiftCode: '', bankAddress: '', currency: 'USD', note: '' });
  const payload = legalEntityProfilePayloadOf(draft(content), 'UTC').profiles[0];
  deepEqual(payload.bankAccounts, source.bankAccounts);
  deepEqual(profileComparisonContentOf(draft(content), 'UTC').bankAccounts, source.bankAccounts);
  content.bankAccounts[0].note = 'SYN 改过';
  equal(source.bankAccounts![0].note, 'SYN 备注');

  const trimmed = legalEntityProfilePayloadOf(
    draft({ bankAccounts: [
      { label: ' ', holderName: ' SYN 户名 ', bankName: ' SYN 银行 ', accountNumber: ' SYN-CNY-02 ', swiftCode: '', bankAddress: ' ', currency: ' CNY ', note: '' },
      { label: '', holderName: '', bankName: '', accountNumber: '', swiftCode: '', bankAddress: '', currency: '', note: '' },
    ] }),
    'UTC',
  ).profiles[0];
  deepEqual(trimmed.bankAccounts, [{ holderName: 'SYN 户名', bankName: 'SYN 银行', accountNumber: 'SYN-CNY-02', currency: 'CNY' }]);
  equal('bankAccounts' in legalEntityProfilePayloadOf(draft({}), 'UTC').profiles[0], false);
  equal('bankAccounts' in profileComparisonContentOf(draft({}), 'UTC'), false);

  const half = draft({
    addressCountry: 'CN', addressLines: 'SYN 地址',
    bankAccounts: [
      { label: '', holderName: 'SYN 户名', bankName: '', accountNumber: 'SYN-01', swiftCode: '', bankAddress: '', currency: '', note: '' },
      { label: '', holderName: '', bankName: '', accountNumber: '', swiftCode: '', bankAddress: '', currency: '', note: '' },
    ],
  });
  deepEqual(legalEntityProfileMissingFields(half), [
    { path: 'profiles[0].bankAccounts[0].bankName', label: '收款账户 1 开户行' },
    { path: 'profiles[0].bankAccounts[0].currency', label: '收款账户 1 币种' },
    { path: 'profiles[0].bankAccounts[1].holderName', label: '收款账户 2 户名' },
    { path: 'profiles[0].bankAccounts[1].bankName', label: '收款账户 2 开户行' },
    { path: 'profiles[0].bankAccounts[1].accountNumber', label: '收款账户 2 账号 / IBAN' },
    { path: 'profiles[0].bankAccounts[1].currency', label: '收款账户 2 币种' },
  ]);
  deepEqual(legalEntityProfileLocalProblems(half, 'UTC'), {});
  const duplicated = legalEntityProfileLocalProblems(
    draft({ bankAccounts: [
      { label: '', holderName: 'A', bankName: 'B', accountNumber: 'SYN-01', swiftCode: '', bankAddress: '', currency: 'CNY', note: '' },
      { label: '', holderName: 'A', bankName: 'B', accountNumber: 'SYN-01', swiftCode: '', bankAddress: '', currency: 'USD', note: '' },
      { label: '', holderName: 'C', bankName: 'D', accountNumber: ' SYN-01 ', swiftCode: '', bankAddress: '', currency: ' CNY', note: '' },
    ] }),
    'UTC',
  );
  deepEqual(duplicated, { 'profiles[0].bankAccounts[2]': ['此币种与账号已经填写'] });

  equal(profileMaintenanceNotes(source).some((note) => note.includes('收款账户')), false);
  deepEqual(profileMaintenanceNotes(revision({ invoicingRegistered: true, invoiceTitle: 'SYN 抬头', representative: { kind: 'LEGAL_REPRESENTATIVE', name: 'SYN' }, contacts: [{ name: 'SYN', email: 'a@example.invalid' }] })), ['未登记收款账户，对账单与发票上印不出收款账户']);
  equal(profileMaintenanceNotes(revision({})).some((note) => note.includes('收款账户')), false);

  deepEqual(bankAccountCurrencyOptions({ countries: new Map(), currencies: new Map([['CNY', '人民币'], ['USD', '美元'], ['SYN', '']]) }), [
    { value: 'CNY', label: '人民币（CNY）' },
    { value: 'USD', label: '美元（USD）' },
    { value: 'SYN', label: 'SYN' },
  ]);
  deepEqual(bankAccountCurrencyOptions({ countries: new Map(), currencies: new Map() }), []);
});

// Covers（票 company-subject-ux/06 第 3 条）：主抬头语言与其他语言的抬头带入再保存原样保全、留空缺席、整行空着不送；
// 半填的行与重复的语言（含主抬头的语言、不分大小写）本地先提；主抬头空着时这两格无处可挂，在开票抬头格下说。
test('多语言抬头：带入与再保存保全、半填与重复语言先提、没有主抬头时说清', () => {
  const source = revision({
    invoicingRegistered: true,
    invoiceTitle: 'SYN 抬头',
    invoiceTitleLanguage: 'zh-Hans',
    invoiceTitles: [{ language: 'en', title: 'SYN Title' }, { language: 'ja', title: 'SYN タイトル' }],
  });
  const content = profileDraftContentOf(source);
  equal(content.invoiceTitleLanguage, 'zh-Hans');
  deepEqual(content.invoiceTitles, source.invoiceTitles);
  const payload = legalEntityProfilePayloadOf(draft(content), 'UTC').profiles[0];
  equal(payload.invoiceTitleLanguage, 'zh-Hans');
  deepEqual(payload.invoiceTitles, source.invoiceTitles);
  const compared = profileComparisonContentOf(draft(content), 'UTC');
  equal(compared.invoiceTitleLanguage, 'zh-Hans');
  deepEqual(compared.invoiceTitles, source.invoiceTitles);
  content.invoiceTitles[0].title = 'SYN 改过';
  equal(source.invoiceTitles![0].title, 'SYN Title');

  const trimmed = legalEntityProfilePayloadOf(
    draft({ invoiceTitle: 'SYN', invoiceTitleLanguage: ' ', invoiceTitles: [{ language: ' en ', title: ' SYN Title ' }, { language: '', title: ' ' }] }),
    'UTC',
  ).profiles[0];
  equal('invoiceTitleLanguage' in trimmed, false);
  deepEqual(trimmed.invoiceTitles, [{ language: 'en', title: 'SYN Title' }]);
  const blank = legalEntityProfilePayloadOf(draft({}), 'UTC').profiles[0];
  equal('invoiceTitleLanguage' in blank, false);
  equal('invoiceTitles' in blank, false);
  // 没有开票资料的旧版带入：两格空着，不从别处补。
  const old = profileDraftContentOf(revision({ invoiceTitleLanguage: 'en', invoiceTitles: [{ language: 'ja', title: 'x' }] }));
  equal(old.invoiceTitleLanguage, '');
  deepEqual(old.invoiceTitles, []);

  deepEqual(
    legalEntityProfileLocalProblems(
      draft({ invoiceTitle: 'SYN', invoiceTitleLanguage: 'zh-Hans', invoiceTitles: [
        { language: 'en', title: '' }, { language: 'ZH-hans', title: 'SYN' }, { language: 'en', title: 'A' }, { language: 'EN', title: 'B' }, { language: '', title: '' },
      ] }),
      'UTC',
    ),
    {
      'profiles[0].invoiceTitles[0]': ['语言与抬头需要成对填写；不需要的行请移除'],
      'profiles[0].invoiceTitles[1]': ['此语言已经填写'],
      'profiles[0].invoiceTitles[3]': ['此语言已经填写'],
    },
  );
  deepEqual(
    legalEntityProfileLocalProblems(draft({ invoiceTitle: ' ', invoiceTitles: [{ language: 'en', title: 'SYN' }] }), 'UTC'),
    { 'profiles[0].invoiceTitle': ['主抬头语言与其他语言的抬头要挂在开票抬头上：先填开票抬头，或清掉它们'] },
  );
  deepEqual(legalEntityProfileLocalProblems(draft({ invoiceTitle: '', invoiceTitleLanguage: 'en' }), 'UTC')['profiles[0].invoiceTitle']?.length, 1);
  deepEqual(legalEntityProfileLocalProblems(draft({ invoiceTitle: '', invoiceTitles: [{ language: '', title: '' }] }), 'UTC'), {});

  equal(languageTagLabel('zh-Hans'), '简体中文（zh-Hans）');
  equal(languageTagLabel(' EN '), '英语（en）');
  equal(languageTagLabel('fr-CA'), 'fr-CA');
  equal(languageTagLabel(''), '');
});

// Covers（票 company-subject-ux/12 第 2 条）：清单主抬头在前、其余照登记次序；主抬头没标语言留空串；没登记开票资料即空表。
test('开票抬头清单：主抬头排第一，其余照登记次序，未登记开票资料为空', () => {
  deepEqual(
    invoiceTitleRowsOf({
      invoicingRegistered: true,
      invoiceTitle: 'SYN 主抬头',
      invoiceTitleLanguage: 'zh-Hans',
      invoiceTitles: [{ language: 'en', title: 'SYN Title' }, { language: 'ja', title: 'SYN 日本語' }],
    }),
    [
      { language: 'zh-Hans', title: 'SYN 主抬头', primary: true },
      { language: 'en', title: 'SYN Title', primary: false },
      { language: 'ja', title: 'SYN 日本語', primary: false },
    ],
  );
  deepEqual(invoiceTitleRowsOf({ invoicingRegistered: true, invoiceTitle: 'SYN 主抬头' }), [{ language: '', title: 'SYN 主抬头', primary: true }]);
  deepEqual(invoiceTitleRowsOf({ invoicingRegistered: false, invoiceTitle: 'SYN 主抬头', invoiceTitles: [{ language: 'en', title: 'SYN' }] }), []);
});
