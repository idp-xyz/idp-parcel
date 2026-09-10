import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import type { CredentialRecord, DutyCollaborationRecord, DutyVerificationRecord } from './api';
import { collaborationRows, credentialRows, credentialUsesLabel, verificationRows } from './register-rows';

// 本文件钉的不是渲染，是三本册子各自**哪一格不得被译顺**（票 sa-cc/10 红线）：凭证的
// 「来源未提供额度」不得显成一个数字；协作事项的两格义务依据不得互相借字段；核对的三轴
// 不得折成一列。夹具取后端行体的字面形状，全是合成串。

const at = '2026-08-14T09:00:00Z';

function credential(over: Partial<CredentialRecord>): CredentialRecord {
  return {
    credential: 'SYN-CRED-01',
    issuer: 'SYN-AUTHORITY-01',
    holder: 'SYN-HOLDER-01',
    procedure: 'SYN-PROC-01',
    validFrom: at,
    validTo: '2026-09-13T09:00:00Z',
    registeredAt: at,
    ...over,
  };
}

// Covers: 0014 自注 / query_credentials.go — uses 缺席是「来源未提供」，不是 0 也不是已用尽；
// 领域把 0 约定为未提供，一次不合契约把 0 送过来也照未提供答，不显成「0 次」。
test('凭证次数额度：缺席与 0 都答来源未提供，有值才显次数', () => {
  equal(credentialUsesLabel(undefined), '来源未提供');
  equal(credentialUsesLabel(0), '来源未提供');
  equal(credentialUsesLabel(3), '3 次');

  const rows = credentialRows([credential({ uses: 3 }), credential({ credential: 'SYN-CRED-02' })]);
  equal(rows.length, 2);
  equal(rows[0].values.uses, '3 次');
  equal(rows[1].values.uses, '来源未提供');
  notEqual(rows[0].key, rows[1].key);
});

// Covers: 登记时间与有效期是三件事——两端合成一段区间显示，登记时刻另占一列，不混用。
test('凭证有效期两端成一段区间，登记时间另列', () => {
  const [row] = credentialRows([credential({ registeredAt: '2026-08-13T08:00:00Z' })]);
  ok(row.values.validity.includes('2026-08-14 09:00:00 UTC'));
  ok(row.values.validity.includes('2026-09-13 09:00:00 UTC'));
  equal(row.values.registeredAt, '2026-08-13 08:00:00 UTC');
});

function collaboration(over: Partial<DutyCollaborationRecord>): DutyCollaborationRecord {
  return {
    scope: 'SYN-UNIT-01',
    kind: 'ASSESSED_DUTY',
    duty: 'SYN-DUTY-01/v1',
    obligor: 'SYN-OBLIGOR-01',
    requirement: 'SYN-ASSESSMENT-01',
    target: 'SYN-DUTY-DESK',
    formedAt: at,
    ...over,
  };
}

// Covers: CONTEXT「税费付款协作事项」— 义务依据两格各显各的字段：核定税费格显税费引用，
// 明确无需付款格显无需付款依据；两格在同一范围上各占一行、键不同。
test('协作事项两格义务依据各显各的字段', () => {
  const rows = collaborationRows([
    collaboration({}),
    collaboration({ kind: 'EXPLICITLY_NOT_REQUIRED', duty: undefined, noPayBasis: 'SYN-PROGRAM-01: no duty' }),
  ]);
  equal(rows[0].values.kind, '已接受监管核定税费');
  equal(rows[0].values.basis, 'SYN-DUTY-01/v1');
  equal(rows[1].values.kind, '明确无需付款依据');
  equal(rows[1].values.basis, 'SYN-PROGRAM-01: no duty');
  notEqual(rows[0].key, rows[1].key);
});

// Covers: 不借字段——核定税费格缺了税费引用不拿无需付款依据顶上，反之亦然；显缺席让不合
// 契约的行露出来，而不是被译成另一格的样子。
test('协作事项缺了本格的字段不借另一格的', () => {
  const [assessed, notRequired] = collaborationRows([
    collaboration({ duty: undefined, noPayBasis: 'stray basis' }),
    collaboration({ kind: 'EXPLICITLY_NOT_REQUIRED', noPayBasis: undefined, duty: 'stray duty' }),
  ]);
  equal(assessed.values.basis, '—');
  equal(notRequired.values.basis, '—');
});

function verification(over: Partial<DutyVerificationRecord>): DutyVerificationRecord {
  return {
    duty: 'SYN-DUTY-01/v1',
    funds: 'SYN-FUNDS-01',
    scope: 'SYN-UNIT-01',
    version: 'digest-v1',
    coverage: 'PARTIAL',
    delta: 'SHORT',
    validity: 'PENDING',
    basis: 'SYN-RULE-01',
    verifiedAt: at,
    ...over,
  };
}

// Covers: ADR-0137 决定三 / CONTEXT 硬句 214 — 三轴三列各译各的词表，行上没有任何合成
// 「状态」列；集外词原样回显，不被译成一句像样的话。
test('核对三轴各占一列、不折总状态，集外词原样回显', () => {
  const [row] = verificationRows([verification({})]);
  equal(row.values.coverage, '部分覆盖');
  equal(row.values.delta, '不足');
  equal(row.values.validity, '待确认');
  deepEqual(
    Object.keys(row.values).filter((key) => /status|settled|paid/i.test(key)),
    [],
  );

  const [odd] = verificationRows([verification({ coverage: 'MOSTLY' })]);
  equal(odd.values.coverage, 'MOSTLY');
});

// Covers: UC-CC-009 — 同三维键的多版本各自成行（迟到事实按新版本追加不覆盖），行键含版本
// 指纹；哪版是当前由读者按核对时间判读，行不代判。
test('同键多版本各自成行，键含版本指纹', () => {
  const rows = verificationRows([
    verification({}),
    verification({ version: 'digest-v2', coverage: 'COVERED', delta: 'NO_DELTA', validity: 'VALID' }),
  ]);
  equal(rows.length, 2);
  notEqual(rows[0].key, rows[1].key);
  ok(rows[0].key.includes('digest-v1') && rows[1].key.includes('digest-v2'));
  equal(rows[1].values.coverage, '已覆盖');
});
