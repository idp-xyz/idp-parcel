import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyRecord } from './api';
import {
  businessPartyFieldPaths,
  businessPartyLocalProblems,
  businessPartyPayloadOf,
  emptyBusinessPartyDraft,
  identificationChanged,
  identificationDraftOf,
  identityCorrectionChanges,
  registeredNumbersChanged,
  suggestedBusinessPartyRevision,
  type BusinessPartyDraft,
} from './business-party-form';

// 本文件钉的是表单只做编码层的事（伞票 admin-write-faces/07 硬句）：修订号编成整数、墙钟时刻换成 RFC 3339、
// 可缺键缺席。载荷逐字镜像 isolated_write_intake.go 的 businessPartyDocument 且**不带 tenantId**；各格**不裁首尾
// 空白**——服务端与受控 CLI 都不裁，表单那层裁了会让两口对同一输入译出不同身份（businessPartyDocument 注释）。

function draft(over: Partial<BusinessPartyDraft>): BusinessPartyDraft {
  return {
    ...emptyBusinessPartyDraft(),
    partyId: 'SYN-PARTY-AGENT-07',
    name: '合成代理七号',
    revision: '1',
    basis: 'SYN-REG-BASIS-BP-07',
    effectiveFrom: '2026-01-02T08:00',
    ...over,
  };
}

// Covers: 各格齐 → businessParties 一项；revision 是 JSON 整数；effectiveFrom 按时区换成 UTC；顶层没有 tenantId。
test('草稿组成载荷：一项、整数修订、UTC 时刻、无租户格', () => {
  const payload = businessPartyPayloadOf(draft({}), 'Asia/Shanghai');
  deepEqual(payload, {
    businessParties: [
      {
        partyId: 'SYN-PARTY-AGENT-07',
        name: '合成代理七号',
        revision: 1,
        basis: 'SYN-REG-BASIS-BP-07',
        effectiveFrom: '2026-01-02T00:00:00Z',
      },
    ],
  });
  equal('tenantId' in payload, false);
});

// Covers: 首尾空白原样带——" X" 与 "X" 在服务端与 CLI 是两个身份，表单不替它们合并；空串照送（那是服务端要点名的格）；
// 生效时刻留空则键缺席，不编成零时刻。
test('空白原样带、空串照送、生效时刻留空缺席', () => {
  const payload = businessPartyPayloadOf(
    draft({ partyId: '  SYN-PARTY-AGENT-07 ', name: '', basis: ' b ', effectiveFrom: '' }),
    'UTC',
  );
  deepEqual(payload.businessParties[0], {
    partyId: '  SYN-PARTY-AGENT-07 ',
    name: '',
    revision: 1,
    basis: ' b ',
  });
});

// Covers: 修订号编不进正整数（空、小数、零、非数字）与时刻换不出来（含 2 月 30 日这种滚过去的假日期）是编码层问题，
// 本地报在它的路径上；其余一律不报。
test('本地只报编码层问题', () => {
  deepEqual(businessPartyLocalProblems(draft({}), 'UTC'), {});
  deepEqual(businessPartyLocalProblems(draft({ revision: '' }), 'UTC'), {
    'businessParties[0].revision': ['修订号要填正整数'],
  });
  deepEqual(businessPartyLocalProblems(draft({ revision: '1.5' }), 'UTC'), {
    'businessParties[0].revision': ['修订号要填正整数'],
  });
  deepEqual(businessPartyLocalProblems(draft({ revision: '0' }), 'UTC'), {
    'businessParties[0].revision': ['修订号要填正整数'],
  });
  deepEqual(businessPartyLocalProblems(draft({ effectiveFrom: 'yesterday' }), 'UTC'), {
    'businessParties[0].effectiveFrom': ['生效时刻要是可解析的日期时间'],
  });
  deepEqual(businessPartyLocalProblems(draft({ effectiveFrom: '2026-02-30T08:00' }), 'UTC'), {
    'businessParties[0].effectiveFrom': ['生效时刻要是可解析的日期时间'],
  });
  // 名称与依据为空不是本地的话：那是服务端构造门要点名的格。
  deepEqual(businessPartyLocalProblems(draft({ name: '', basis: '' }), 'UTC'), {});
});

// Covers: 修订号编不进整数时载荷不造假数——那一格缺席，由问题表拦住不送。
test('修订号编不进整数时载荷缺席该格', () => {
  const payload = businessPartyPayloadOf(draft({ revision: 'x' }), 'UTC');
  equal('revision' in payload.businessParties[0], false);
});

// Covers: 建议修订号——标识在已取回列表里 → 最新修订 + 1；不在 → 1；标识空 → 1；列表没取到（null）一律 1。
// 查找按原串比、不裁空白：载荷不裁，" SYN-PARTY-01" 送上去就是另一个身份，建议也得按它是新标识算。
test('建议修订号取列表最新修订加一，不在册或列表未取到为一', () => {
  const rows: BusinessPartyRecord[] = [
    {
      tenantId: 'SYN-TENANT-01',
      partyId: 'SYN-PARTY-SHIPPER-01',
      partyName: '合成货主一号',
      status: 'EFFECTIVE',
      revision: 3,
      basis: 'b',
      effectiveFrom: '2026-01-02T00:00:00Z',
      registeredAt: '2026-01-02T00:00:00Z',
      categories: ['NO_ROLE'], relationshipCounts: { asHolder: { live: 0, candidate: 0 }, asCounterparty: { live: 0, candidate: 0 } },
    },
  ];
  equal(suggestedBusinessPartyRevision(rows, 'SYN-PARTY-SHIPPER-01'), 4);
  equal(suggestedBusinessPartyRevision(rows, ' SYN-PARTY-SHIPPER-01'), 1);
  equal(suggestedBusinessPartyRevision(rows, 'SYN-PARTY-99'), 1);
  equal(suggestedBusinessPartyRevision(rows, ''), 1);
  equal(suggestedBusinessPartyRevision([], 'SYN-PARTY-SHIPPER-01'), 1);
  equal(suggestedBusinessPartyRevision(null, 'SYN-PARTY-SHIPPER-01'), 1);
});

// Covers: 表单认领的路径表与载荷键一一对应，键名逐字取 businessPartyDocument 的 json 标签。
test('认领的路径表', () => {
  deepEqual([...businessPartyFieldPaths], [
    'businessParties[0].partyId',
    'businessParties[0].name',
    'businessParties[0].revision',
    'businessParties[0].basis',
    'businessParties[0].effectiveFrom',
    'businessParties[0].registrationCountry',
    'businessParties[0].lifetimeRegistrationNumbers',
    'businessParties[0].identityCorrectionBasis',
    'businessParties[0].externalReferences',
  ]);
});

// Covers: ADR-0367 识别信息的编码——没填的格缺席、空行不带、填了一半的行原样送（由服务端点名）；不裁空白。
test('识别信息进载荷：空格缺席、空行不带', () => {
  const bare = businessPartyPayloadOf(draft({}), 'UTC').businessParties[0];
  equal('registrationCountry' in bare, false);
  equal('lifetimeRegistrationNumbers' in bare, false);
  equal('identityCorrectionBasis' in bare, false);
  equal('externalReferences' in bare, false);
  const item = businessPartyPayloadOf(
    draft({
      registrationCountry: 'CN',
      lifetimeNumbers: [{ typeCode: 'USCC', number: '91000000000000009X' }, { typeCode: '', number: '' }],
      identityCorrectionBasis: 'SYN-FIX',
      externalReferences: [{ system: 'ERP', purpose: '', code: 'C-1' }, { system: '', purpose: '', code: '' }],
    }),
    'UTC',
  ).businessParties[0];
  equal(item.registrationCountry, 'CN');
  deepEqual(item.lifetimeRegistrationNumbers, [{ typeCode: 'USCC', number: '91000000000000009X' }]);
  equal(item.identityCorrectionBasis, 'SYN-FIX');
  deepEqual(item.externalReferences, [{ system: 'ERP', purpose: '', code: 'C-1' }]);
});

// Covers: 更正时把册上的识别信息摆回草稿；改没改、改没改号按内容比，与次序无关；册上没号时补登不算改号。
test('识别信息的预填与改动判读', () => {
  const row = {
    identityLayerRegistered: true,
    registrationCountry: 'CN',
    lifetimeRegistrationNumbers: [{ typeCode: 'A', number: '1' }, { typeCode: 'B', number: '2' }],
    externalReferences: [{ system: 'ERP', purpose: 'CUSTOMER', code: 'C-1' }],
  };
  const prefilled = draft(identificationDraftOf(row));
  deepEqual(prefilled.lifetimeNumbers, row.lifetimeRegistrationNumbers);
  equal(prefilled.identityCorrectionBasis, '');
  equal(identificationChanged(row, prefilled), false);
  equal(registeredNumbersChanged(row, prefilled), false);
  const reordered = draft({ ...prefilled, lifetimeNumbers: [...prefilled.lifetimeNumbers].reverse() });
  equal(identificationChanged(row, reordered), false);
  const recoded = draft({ ...prefilled, externalReferences: [{ system: 'ERP', purpose: 'CUSTOMER', code: 'C-2' }] });
  equal(identificationChanged(row, recoded), true);
  equal(registeredNumbersChanged(row, recoded), false);
  const removed = draft({ ...prefilled, registrationCountry: '', lifetimeNumbers: [] });
  equal(registeredNumbersChanged(row, removed), true);
  equal(registeredNumbersChanged({}, draft({ registrationCountry: 'CN', lifetimeNumbers: [{ typeCode: 'A', number: '1' }] })), false);
});

// Covers: 「更正登记信息」保存前核对的那几行（票 business-party-redesign/11）——预填后不动一行都没有；号与编码按内容比、与次序
// 无关，没填的空行不算；改了的只列改了的那几格，值是将要送上去的（去首尾空白）。
test('identityCorrectionChanges 只列有改动的格', () => {
  const row = {
    identityLayerRegistered: true,
    registrationCountry: 'CN',
    lifetimeRegistrationNumbers: [{ typeCode: 'A', number: '1' }, { typeCode: 'B', number: '2' }],
    externalReferences: [{ system: 'ERP', purpose: 'CUSTOMER', code: 'C-1' }],
  };
  const prefilled = draft({ ...identificationDraftOf(row), name: '合成代理七号' });
  deepEqual(identityCorrectionChanges(row, '合成代理七号', prefilled), []);
  const reshuffled = draft({
    ...prefilled,
    lifetimeNumbers: [...prefilled.lifetimeNumbers].reverse(),
    externalReferences: [...prefilled.externalReferences, { system: '', purpose: '', code: '' }],
  });
  deepEqual(identityCorrectionChanges(row, '合成代理七号', reshuffled), []);
  const corrected = draft({
    ...prefilled,
    name: ' 合成代理七号（更名） ',
    lifetimeNumbers: [{ typeCode: 'A', number: '1' }],
    externalReferences: [],
  });
  deepEqual(identityCorrectionChanges(row, '合成代理七号', corrected), [
    { field: '名称', before: '合成代理七号', after: '合成代理七号（更名）' },
    { field: '注册号', before: 'A 1；B 2', after: 'A 1' },
    { field: '外部系统编码', before: 'ERP · CUSTOMER C-1', after: '' },
  ]);
});
