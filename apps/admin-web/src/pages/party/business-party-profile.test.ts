import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyProfileRevisionRecord } from './api';
import {
  businessPartyProfileChanged,
  businessPartyProfileDraftBasedOn,
  businessPartyProfileLocalProblems,
  businessPartyProfileMissingFields,
  businessPartyProfilePayloadOf,
  businessPartyProfileVersionFact,
  customerLacksContacts,
  effectiveBusinessPartyProfile,
  emptyBusinessPartyProfileDraft,
  otherLanguageNameOf,
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
