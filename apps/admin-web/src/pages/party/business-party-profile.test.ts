import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyProfileRevisionRecord } from './api';
import { languageTagLabel } from './legal-entity-profile';
import {
  businessPartyProfileChanged,
  businessPartyProfileComparisonOf,
  businessPartyProfileDraftBasedOn,
  businessPartyProfileLocalProblems,
  businessPartyProfileMissingFields,
  businessPartyProfilePayloadOf,
  businessPartyProfileStateOf,
  businessPartyProfileVersionFact,
  customerLacksContacts,
  effectiveBusinessPartyProfile,
  emptyBusinessPartyProfileDraft,
  otherLanguageNameOf,
  profileClosedLine,
  profileContactsEmptyNoteOf,
  profileEditTagText,
  profileSummaryEmptyNoteOf,
  profileVersionsEmptyNoteOf,
  suggestedBusinessPartyProfileRevision,
} from './business-party-profile';

const record: BusinessPartyProfileRevisionRecord = {
  tenantId: 'SYN-TENANT-01',
  partyId: 'SYN-PARTY-SHIPPER-01',
  revision: 1,
  basis: 'SYN-BASIS',
  effectiveFrom: '2026-01-01T00:00:00Z',
  registeredAt: '2026-01-01T00:00:00Z',
  names: [{ language: 'en', name: 'SYN Shipper One' }],
  correspondenceAddress: { country: 'CN', lines: ['上海市 SYN 路 1 号'] },
  taxRegistrationNumbers: [],
  invoicingRegistered: true,
  invoiceTitle: '合成货主一号',
  contacts: [{ name: '林悦', email: 'finance@example.invalid', purposes: ['FINANCE'], primaryFor: ['FINANCE'] }],
};

// Covers: 载荷换主体格、不带法定代表人与注册地址（没填时）、带其他语言名称，空行不送；与法人资料同形的各格照带。
test('往来单位资料的载荷', () => {
  const draft = { ...emptyBusinessPartyProfileDraft('SYN-PARTY-01'), basis: 'SYN-B', effectiveFrom: '2026-03-01T08:00', revision: '1' };
  draft.names = [{ language: 'en', name: 'SYN One' }, { language: '', name: '' }];
  draft.correspondenceCountry = 'CN';
  draft.correspondenceLines = '上海市\n\nSYN 路';
  const item = businessPartyProfilePayloadOf(draft, 'Asia/Shanghai').profiles[0];
  equal(item.partyId, 'SYN-PARTY-01');
  equal('legalEntityId' in item, false);
  equal('representative' in item, false);
  equal('registeredAddress' in item, false);
  deepEqual(item.names, [{ language: 'en', name: 'SYN One' }]);
  deepEqual(item.correspondenceAddress, { country: 'CN', lines: ['上海市', 'SYN 路'] });
  equal(item.effectiveFrom, '2026-03-01T00:00:00Z');
});

// Covers: 注册地址不算必填；来源单据与生效时间照旧必填；名称半行、语言重复本地先提。
test('往来单位资料的必填与本地问题', () => {
  const missing = businessPartyProfileMissingFields(emptyBusinessPartyProfileDraft('P')).map((field) => field.label);
  deepEqual(missing, ['来源单据', '生效时间']);
  const draft = { ...emptyBusinessPartyProfileDraft('P'), revision: '1' };
  draft.names = [{ language: 'en', name: '' }, { language: 'en', name: 'A' }, { language: 'EN', name: 'B' }];
  const problems = businessPartyProfileLocalProblems(draft, 'UTC');
  equal(problems['profiles[0].names[0]']?.[0], '语言与名称需要成对填写；不需要的行请移除');
  equal(problems['profiles[0].names[1]'], undefined);
  equal(problems['profiles[0].names[2]']?.[0], '此语言已经填写');
  equal(problems['profiles[0].registeredAddress'], undefined);
  const halfAddress = businessPartyProfileLocalProblems({ ...draft, names: [], addressCountry: 'CN' }, 'UTC');
  equal(halfAddress['profiles[0].registeredAddress']?.[0], '国家 / 地区与地址需要一起填写');
});

// Covers: 预填基于此刻有效的一版（服务端答的修订号），三格「这一次更新」不沿用；没改内容判为未改，改了判为改了；修订号建议最大加一。
test('往来单位资料的预填与改动判读', () => {
  const body = { outcome: 'X', partyId: 'SYN-PARTY-SHIPPER-01', effectiveRevision: 1, revisions: [record, { ...record, revision: 2, effectiveFrom: '2027-01-01T00:00:00Z' }] };
  const effective = effectiveBusinessPartyProfile(body);
  equal(effective?.revision, 1);
  equal(effectiveBusinessPartyProfile({ ...body, effectiveRevision: undefined }), null);
  equal(suggestedBusinessPartyProfileRevision(body.revisions), 3);
  equal(suggestedBusinessPartyProfileRevision(null), 1);
  const base = businessPartyProfileDraftBasedOn('SYN-PARTY-SHIPPER-01', effective);
  equal(base.basis, '');
  equal(base.invoiceTitle, '合成货主一号');
  equal(base.correspondenceLines, '上海市 SYN 路 1 号');
  equal(base.addressCountry, '');
  deepEqual(base.names, [{ language: 'en', name: 'SYN Shipper One' }]);
  equal(businessPartyProfileChanged(base, { ...base, basis: 'new', effectiveFrom: '2026-05-01T08:00' }, 'UTC'), false);
  equal(businessPartyProfileChanged(base, { ...base, names: [{ language: 'en', name: 'Renamed' }] }, 'UTC'), true);
});

// Covers: 抬头名称下那一行先取英文（含 en-GB 这类地区写法），没有英文取第一个；没有资料、没有名称或名称全空都不显。
test('otherLanguageNameOf 先取英文', () => {
  equal(otherLanguageNameOf(record), 'SYN Shipper One');
  equal(otherLanguageNameOf({ names: [{ language: 'ja', name: 'SYN 日本' }, { language: 'en-GB', name: 'SYN UK' }] }), 'SYN UK');
  equal(otherLanguageNameOf({ names: [{ language: 'ja', name: 'SYN 日本' }, { language: 'eng', name: 'SYN ENG' }] }), 'SYN 日本');
  equal(otherLanguageNameOf({ names: [{ language: 'en', name: '  ' }] }), null);
  equal(otherLanguageNameOf({}), null);
  equal(otherLanguageNameOf(null), null);
});

// Covers: 只对此刻是客户的单位提示；没有资料与资料里没有联系人都算缺，有一位就不提示；别的角色不提示。
test('customerLacksContacts 只提示缺联系人的客户', () => {
  equal(customerLacksContacts(['CUSTOMER'], null), true);
  equal(customerLacksContacts(['CUSTOMER'], { ...record, contacts: [] }), true);
  equal(customerLacksContacts(['CUSTOMER', 'SUPPLIER'], record), false);
  equal(customerLacksContacts(['CARRIER_AGENT'], null), false);
  equal(customerLacksContacts([], { ...record, contacts: [] }), false);
});

// Covers: 头部「资料」格分清在读、读不到、未登记、尚无生效版本与第几版；读不到不冒充未登记，别家的答复不认。
test('businessPartyProfileVersionFact 分清没登与读不到', () => {
  const party = 'SYN-PARTY-SHIPPER-01';
  const answer = (effectiveRevision: number | undefined, revisions: BusinessPartyProfileRevisionRecord[], partyId = party) =>
    ({ kind: 'outcome', status: 200, body: { outcome: 'X', partyId, effectiveRevision, revisions } }) as const;
  equal(businessPartyProfileVersionFact(null, party), '…');
  equal(businessPartyProfileVersionFact({ kind: 'transport', message: 'down' }, party), '读不到');
  equal(businessPartyProfileVersionFact(answer(undefined, []), party), '未登记');
  equal(businessPartyProfileVersionFact(answer(undefined, [{ ...record, effectiveFrom: '2099-01-01T00:00:00Z' }]), party), '尚无生效版本');
  equal(businessPartyProfileVersionFact(answer(1, [record]), party), '第 1 版');
  equal(businessPartyProfileVersionFact(answer(1, [record], 'SYN-PARTY-OTHER'), party), '…');
});

// Covers: 资料状态分清在读、读不到、别家的答复（当作在读）、没登过、登过却都在未来生效、有生效版；有生效版交出的就是那一版。
test('businessPartyProfileStateOf 分清在读、空着与有生效版', () => {
  const party = 'SYN-PARTY-SHIPPER-01';
  const answer = (effectiveRevision: number | undefined, revisions: BusinessPartyProfileRevisionRecord[], partyId = party) =>
    ({ kind: 'outcome', status: 200, body: { outcome: 'X', partyId, effectiveRevision, revisions } }) as const;
  deepEqual(businessPartyProfileStateOf(null, party), { kind: 'note', text: '正在读取资料…' });
  deepEqual(businessPartyProfileStateOf({ kind: 'transport', message: 'down' }, party), { kind: 'note', text: '资料暂时读不到，稍后刷新再看。' });
  deepEqual(businessPartyProfileStateOf(answer(1, [record], 'SYN-PARTY-OTHER'), party), { kind: 'note', text: '正在读取资料…' });
  const unregistered = businessPartyProfileStateOf(answer(undefined, []), party);
  deepEqual([unregistered.kind, unregistered.kind === 'empty' && unregistered.scheduled], ['empty', false]);
  const scheduled = businessPartyProfileStateOf(answer(undefined, [{ ...record, effectiveFrom: '2099-01-01T00:00:00Z' }]), party);
  deepEqual([scheduled.kind, scheduled.kind === 'empty' && scheduled.scheduled], ['empty', true]);
  const present = businessPartyProfileStateOf(answer(1, [record, { ...record, revision: 2, effectiveFrom: '2099-01-01T00:00:00Z' }]), party);
  deepEqual([present.kind, present.kind === 'present' && present.record.revision], ['present', 1]);
});

// Covers: 资料空着时各处的话——概要里没登过与都在未来生效各一句，是客户才补建议那一行；联系人签按「有资料没联系人 / 没有生效的
// 资料 / 没登过」各一个标题、同一句去处。登过停用的往来单位（末一个参数为假）登记册不再收新的资料：去处那一句换掉，
// 对客户的建议不提，资料版本也不再说「登记后」。
test('资料空着时的标题与说明', () => {
  deepEqual(profileSummaryEmptyNoteOf(false, ['SUPPLIER'], true), {
    title: '还没有登记资料',
    lines: ['联系人、地址、税号与收款账户都在「更新资料」里登记。'],
  });
  deepEqual(profileSummaryEmptyNoteOf(false, ['CUSTOMER'], true), {
    title: '还没有登记资料',
    lines: ['联系人、地址、税号与收款账户都在「更新资料」里登记。', '这家单位是客户，建议登记一位联系人，下单和对账时找得到人。'],
  });
  deepEqual(profileSummaryEmptyNoteOf(true, [], true), { title: '还没有生效的资料', lines: ['已登记的版本都在未来生效。'] });
  const body = { outcome: 'X', partyId: record.partyId, revisions: [record] };
  const lines = ['联系人与地址、税号一起在「更新资料」里登记。'];
  deepEqual(profileContactsEmptyNoteOf({ kind: 'present', record: { ...record, contacts: [] }, body }, true), { title: '还没有登记联系人', lines });
  deepEqual(profileContactsEmptyNoteOf({ kind: 'empty', scheduled: true, body }, true), { title: '还没有生效的资料', lines });
  deepEqual(profileContactsEmptyNoteOf({ kind: 'empty', scheduled: false, body: { ...body, revisions: [] } }, true), { title: '还没有登记资料', lines });
  deepEqual(profileVersionsEmptyNoteOf(true), { title: '还没有登记资料', lines: ['登记后每一版都列在这里，旧版本不会被覆盖。'] });
  // 登过停用的
  deepEqual(profileSummaryEmptyNoteOf(false, ['CUSTOMER'], false), { title: '还没有登记资料', lines: [profileClosedLine] });
  deepEqual(profileSummaryEmptyNoteOf(true, [], false), { title: '还没有生效的资料', lines: ['已登记的版本都在未来生效。', profileClosedLine] });
  deepEqual(profileContactsEmptyNoteOf({ kind: 'present', record: { ...record, contacts: [] }, body }, false), {
    title: '还没有登记联系人',
    lines: [profileClosedLine],
  });
  deepEqual(profileVersionsEmptyNoteOf(false), { title: '还没有登记资料', lines: [profileClosedLine] });
});

// Covers: 保存前核对的比较（票 business-party-redesign/11）——预填后不动，没有一行有变化；其他语言名称按语言比、排在最前：
// 名称变了是修改、多一种语言是新增、去掉是移除，整行空着的不算；内容各格的改动借法人资料的比较，排在名称之后。
test('businessPartyProfileComparisonOf：其他语言名称排在最前，内容各格借法人资料的比较', () => {
  const untouched = businessPartyProfileDraftBasedOn(record.partyId, record);
  const changedOf = (draft: typeof untouched) =>
    businessPartyProfileComparisonOf(record, draft, 'UTC')
      .fields.filter((field) => field.kind !== 'unchanged')
      .map((field) => [field.field, field.kind, field.before, field.after]);
  deepEqual(changedOf(untouched), []);

  const renamed = {
    ...untouched,
    names: [
      { language: 'en', name: ' SYN Shipper 1 ' },
      { language: 'ja', name: 'SYN 荷主' },
      { language: '', name: '' },
    ],
  };
  deepEqual(changedOf(renamed), [
    [`其他语言名称 · ${languageTagLabel('en')}`, 'modified', 'SYN Shipper One', 'SYN Shipper 1'],
    [`其他语言名称 · ${languageTagLabel('ja')}`, 'added', '', 'SYN 荷主'],
  ]);

  const dropped = { ...untouched, names: [], invoiceTitle: '合成货主一号有限公司' };
  deepEqual(changedOf(dropped), [
    [`其他语言名称 · ${languageTagLabel('en')}`, 'removed', 'SYN Shipper One', ''],
    ['开票抬头', 'modified', '合成货主一号', '合成货主一号有限公司'],
  ]);
});

// Covers: 编辑态页头那枚标记——有生效版说在第几版的基础上改；基于的那一版还没生效时说清；一版都没有是登记第一版。
test('profileEditTagText 说清在哪一版的基础上改', () => {
  equal(profileEditTagText(null, false), '正在登记资料 · 第 1 版');
  equal(profileEditTagText({ revision: 2 }, false), '正在更新资料 · 在第 2 版的基础上改');
  equal(profileEditTagText({ revision: 3 }, true), '正在更新资料 · 在尚未生效的第 3 版的基础上改');
});
