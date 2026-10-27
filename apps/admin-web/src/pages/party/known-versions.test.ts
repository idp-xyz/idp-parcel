import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type {
  CommercialPolicyListResponseBody,
  CustomerContractListResponseBody,
  ServiceProductListResponseBody,
  SupplierAgreementListResponseBody,
} from './api';
import {
  agreementKnownVersions,
  contractKnownVersions,
  policyKnownVersions,
  productKnownVersions,
  versionHintLines,
  versionHintOf,
  type KnownVersion,
} from './known-versions';

// 演示种子里供应商协议：TRUNK 三版（v1、v2 已退役，v3 在用）夹着 LASTMILE 一版。
const known: KnownVersion[] = [
  { objectId: 'SYN-TRUNK', version: 'v1', status: 'RETIRED', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-LASTMILE', version: 'v1', status: 'EFFECTIVE', effectiveStartsAt: '2026-01-01T00:00:00Z' },
  { objectId: 'SYN-TRUNK', version: 'v3', status: 'EFFECTIVE', effectiveStartsAt: '2026-06-01T00:00:00Z' },
  { objectId: 'SYN-TRUNK', version: 'v2', status: 'RETIRED', effectiveStartsAt: '2026-03-01T00:00:00Z' },
];

const base = { objectId: 'SYN-TRUNK', version: '', startsAt: null, endsAt: null };

test('标识对上已有对象：列出各版，新的在前，在用的那版标出来', () => {
  const hint = versionHintOf(known, base)!;
  deepEqual(hint.versions.map((row) => row.version), ['v3', 'v2', 'v1']);
  deepEqual(versionHintLines(hint, 'SYN-TRUNK'), [
    { tone: 'info', text: '「SYN-TRUNK」已有版本：v3（在用）、v2（已退役）、v1（已退役）。' },
  ]);
});

test('标识对不上任何对象：说这会是第一版，请核对标识', () => {
  const lines = versionHintLines(versionHintOf(known, { ...base, objectId: 'SYN-TRUNK-X' })!, 'SYN-TRUNK-X');
  equal(lines.length, 1);
  equal(lines[0].text.includes('第一版'), true);
});

test('正在敲的标识是已有标识的前缀，不报第一版，免得敲一个字闪一下', () => {
  const hint = versionHintOf(known, { ...base, objectId: 'SYN-TR' })!;
  equal(hint.typingKnownPrefix, true);
  deepEqual(versionHintLines(hint, 'SYN-TR'), []);
});

test('标识空着不提示', () => {
  equal(versionHintOf(known, { ...base, objectId: '  ' }), null);
});

test('版本号与已有的某一版相同：报撞号', () => {
  const lines = versionHintLines(versionHintOf(known, { ...base, version: ' v2 ' })!, 'SYN-TRUNK');
  equal(lines.some((line) => line.tone === 'warn' && line.text.startsWith('版本号 v2 已经发布过')), true);
  equal(versionHintLines(versionHintOf(known, { ...base, version: 'v4' })!, 'SYN-TRUNK').some((line) => line.tone === 'warn'), false);
});

test('区间与在用旧版重叠：报；旧版已退役的不报；同号那一版由撞号说、不再报重叠', () => {
  // v3 在用、持续有效；新版从 2026-09-01 起，重叠。
  const overlapped = versionHintOf(known, { ...base, version: 'v4', startsAt: '2026-09-01T00:00:00Z' })!;
  deepEqual(overlapped.overlaps.map((row) => row.version), ['v3']);
  equal(versionHintLines(overlapped, 'SYN-TRUNK').some((line) => line.text.includes('适用冲突')), true);
  // 同号：v3 不再算旧版。
  deepEqual(versionHintOf(known, { ...base, version: 'v3', startsAt: '2026-09-01T00:00:00Z' })!.overlaps, []);
});

test('区间首尾相接不算重叠；新版有止点且落在旧版开始之前也不重叠', () => {
  const closed: KnownVersion[] = [
    { objectId: 'P', version: 'v1', status: 'EFFECTIVE', effectiveStartsAt: '2026-01-01T00:00:00Z', effectiveEndsAt: '2026-06-01T00:00:00Z' },
  ];
  const input = { objectId: 'P', version: 'v2', endsAt: null };
  deepEqual(versionHintOf(closed, { ...input, startsAt: '2026-06-01T00:00:00Z' })!.overlaps, []);
  deepEqual(versionHintOf(closed, { ...input, startsAt: '2026-05-31T23:59:59Z' })!.overlaps.map((row) => row.version), ['v1']);
  deepEqual(versionHintOf(closed, { objectId: 'P', version: 'v2', startsAt: '2025-01-01T00:00:00Z', endsAt: '2026-01-01T00:00:00Z' })!.overlaps, []);
});

test('已计划生效的版本也算还在参与解析；起点没填或换不成时刻不判重叠', () => {
  const planned: KnownVersion[] = [
    { objectId: 'P', version: 'v1', status: 'PUBLISHED', effectiveStartsAt: '2030-01-01T00:00:00Z' },
  ];
  deepEqual(versionHintOf(planned, { objectId: 'P', version: 'v2', startsAt: '2031-01-01T00:00:00Z', endsAt: null })!.overlaps.map((row) => row.version), ['v1']);
  deepEqual(versionHintOf(planned, { objectId: 'P', version: 'v2', startsAt: null, endsAt: null })!.overlaps, []);
  deepEqual(versionHintOf(planned, { objectId: 'P', version: 'v2', startsAt: 'not-a-time', endsAt: null })!.overlaps, []);
});

test('读面没有状态的册只列版本、查撞号，不报重叠（判不出旧版是否已收尾）', () => {
  const noStatus: KnownVersion[] = [{ objectId: 'C', version: 'v1', effectiveStartsAt: '2026-01-01T00:00:00Z' }];
  const hint = versionHintOf(noStatus, { objectId: 'C', version: 'v2', startsAt: '2026-09-01T00:00:00Z', endsAt: null })!;
  deepEqual(hint.overlaps, []);
  deepEqual(versionHintLines(hint, 'C'), [{ tone: 'info', text: '「C」已有版本：v1。' }]);
});

test('政策册响应按册判别：带状态的给状态，没有状态的不给，随所属版本发布的两本给空', () => {
  const credit = {
    outcome: 'COMMERCIAL_POLICIES_LISTED',
    kind: 'CREDIT_POLICY',
    policies: [
      {
        objectId: 'CP',
        version: 'v1',
        legalEntity: 'LE',
        authorityLevel: 'L',
        chargeType: 'C',
        limitMinor: 0,
        effectiveStartsAt: '2026-01-01T00:00:00Z',
        registeredAt: '2026-01-01T00:00:00Z',
      },
    ],
  } satisfies CommercialPolicyListResponseBody;
  deepEqual(policyKnownVersions(credit), [{ objectId: 'CP', version: 'v1', effectiveStartsAt: '2026-01-01T00:00:00Z', effectiveEndsAt: undefined }]);
  const authorization = {
    outcome: 'COMMERCIAL_POLICIES_LISTED',
    kind: 'AUTHORIZATION_RULE',
    policies: [
      {
        objectId: 'AR',
        version: 'v2',
        scope: 'S',
        status: 'EFFECTIVE',
        effectiveStartsAt: '2026-01-01T00:00:00Z',
        publishedAt: '2026-01-01T00:00:00Z',
        cancellationAuthorityDeclared: false,
        cancellationAuthorities: [],
      },
    ],
  } satisfies CommercialPolicyListResponseBody;
  equal(policyKnownVersions(authorization)[0].status, 'EFFECTIVE');
  deepEqual(policyKnownVersions({ outcome: 'COMMERCIAL_POLICIES_LISTED', kind: 'AS_OF_POLICY', policies: [] }), []);
});

test('被截止的协议版本按截止点算区间：截止后不再与之后的新版重叠', () => {
  const body = {
    outcome: 'SUPPLIER_AGREEMENTS_LISTED',
    agreements: [
      {
        objectId: 'SA',
        version: 'v1',
        scope: 'S',
        status: 'EFFECTIVE',
        effectiveStartsAt: '2026-01-01T00:00:00Z',
        publishedAt: '2026-01-01T00:00:00Z',
        contentRegistered: false,
        selectionEndsAt: '2026-06-01T00:00:00Z',
      },
    ],
  } satisfies SupplierAgreementListResponseBody;
  const known = agreementKnownVersions(body);
  equal(known[0].effectiveEndsAt, '2026-06-01T00:00:00Z');
  equal(known[0].scope, 'S');
  const hint = versionHintOf(known, { objectId: 'SA', version: 'v2', startsAt: '2026-06-01T00:00:00Z', endsAt: null })!;
  deepEqual(hint.overlaps, []);
});

// 票 demo-chain-gaps/184：另五本带状态的目录也回显截止点，已知版本取 selectionEndsAt ?? effectiveEndsAt，与协议同口径。
const closedRow = {
  objectId: 'OBJ',
  version: 'v1',
  scope: 'S',
  status: 'EFFECTIVE',
  effectiveStartsAt: '2026-01-01T00:00:00Z',
  effectiveEndsAt: '2026-12-31T00:00:00Z',
  publishedAt: '2026-01-01T00:00:00Z',
  selectionEndsAt: '2026-06-01T00:00:00Z',
};
const fiveCatalogues: Array<[string, () => KnownVersion[]]> = [
  [
    '服务产品',
    () => productKnownVersions({ outcome: 'SERVICE_PRODUCTS_LISTED', products: [closedRow] } as unknown as ServiceProductListResponseBody),
  ],
  [
    '客户合同',
    () => contractKnownVersions({ outcome: 'CUSTOMER_CONTRACTS_LISTED', contracts: [closedRow] } as unknown as CustomerContractListResponseBody),
  ],
  ...(['AUTHORIZATION_RULE', 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY', 'CUSTOMER_SERVICE_RULE'] as const).map(
    (kind): [string, () => KnownVersion[]] => [
      kind,
      () => policyKnownVersions({ outcome: 'COMMERCIAL_POLICIES_LISTED', kind, policies: [closedRow] } as unknown as CommercialPolicyListResponseBody),
    ],
  ),
];
for (const [name, build] of fiveCatalogues) {
  test(`被截止的${name}版本按截止点算区间：原区间不被覆盖，截止后不再与新版重叠`, () => {
    const known = build();
    equal(known[0].effectiveEndsAt, '2026-06-01T00:00:00Z');
    const hint = versionHintOf(known, { objectId: 'OBJ', version: 'v2', startsAt: '2026-06-01T00:00:00Z', endsAt: null })!;
    deepEqual(hint.overlaps, []);
  });
}
