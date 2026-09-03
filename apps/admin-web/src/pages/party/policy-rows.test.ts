import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { CommercialPolicyListResponseBody } from './api';
import { cancellationCell, kindColumns, rowsOf } from './policy-rows';
import {
  commercialPolicyKinds,
  policyKindLabels,
  policyKindSources,
  registrationSnapshotHints,
} from './presentation';

// 输入取后端 query_commercial_catalogue_test.go 里
// TestPoliciesEndpointTranscribesCreditLimitAsExactlyOneKey 钉住的两行：零金额（开放结束）与比例
// （带结束）。额度两键恰一在场是后端契约，零额度是合法声明——前端不得把 0 显示成缺席。
const creditPolicies: CommercialPolicyListResponseBody = {
  outcome: 'COMMERCIAL_POLICIES_LISTED',
  kind: 'CREDIT_POLICY',
  policies: [
    {
      objectId: 'credit-zero',
      version: 'v1',
      legalEntity: 'legal-1',
      authorityLevel: 'level-commercial',
      chargeType: 'charge-freight',
      limitMinor: 0,
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      registeredAt: '2026-01-01T00:00:00Z',
    },
    {
      objectId: 'credit-ratio',
      version: 'v1',
      legalEntity: 'legal-1',
      authorityLevel: 'level-commercial',
      chargeType: 'charge-freight',
      limitRatioBasisPoints: 1500,
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      effectiveEndsAt: '2026-01-01T01:00:00Z',
      registeredAt: '2026-01-01T00:00:00Z',
    },
  ],
};

test('信用政策是第七本册：chip 词表与列集都有它', () => {
  ok(commercialPolicyKinds.includes('CREDIT_POLICY'));
  equal(policyKindLabels.CREDIT_POLICY, '信用政策');
  deepEqual(
    kindColumns.CREDIT_POLICY.map((column) => column.header),
    ['政策对象 / 版本', '责任法人', '授权层级', '费用类型', '额度', '有效区间', '登记时间'],
  );
});

// 票 admin-write-faces/03：册与发布口对象类别是两条分类轴，页面不对齐词表而是逐册写清「谁喂它」。
// 钉两件：每本册都有那一句；两对近形不同词（PRICE_POLICY 册 ← PRICE_RULE 类别、
// PRE_ACCEPTANCE_CONTROL 册 ← 挂 CUSTOMER_CONTRACT 版本的声明）在句子里点名了对方的词，
// 少了哪一句操作者就会照 chip 抄词进发布快照。
test('每本册都写明由发布口的哪一类版本或哪个声明通道喂入', () => {
  for (const kind of commercialPolicyKinds) {
    ok(policyKindSources[kind].trim().length > 0, `${kind} 缺「谁喂它」那一句`);
  }
  ok(policyKindSources.PRICE_POLICY.includes('PRICE_RULE'));
  ok(policyKindSources.PRE_ACCEPTANCE_CONTROL.includes('CUSTOMER_CONTRACT'));
  ok(policyKindSources.AS_OF_POLICY.includes('ACCEPTANCE_RULE_PACKAGE'));
});

test('发布签的提示句把九个对象类别词与「两条分类轴」都说出来', () => {
  const hint = registrationSnapshotHints.publication;
  for (const word of [
    'SERVICE_PRODUCT',
    'CUSTOMER_CONTRACT',
    'SUPPLIER_AGREEMENT',
    'ACCEPTANCE_RULE_PACKAGE',
    'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
    'PRICE_RULE',
    'SETTLEMENT_POLICY',
    'CREDIT_POLICY',
    'AUTHORIZATION_RULE',
  ]) {
    ok(hint.includes(word), `提示句缺对象类别 ${word}`);
  }
  ok(hint.includes('两条分类轴'));
  // 此前的理由（翻译属渠道接入契约、随 PAR-INT-01 提供）已被 ADR-0101 收窄为只适用客户渠道，
  // 操作者面不得再这样说。
  ok(!hint.includes('PAR-INT-01'));
});

test('零金额额度上列为金额 0，不是「未声明」也不是缺席', () => {
  const [zero] = rowsOf(creditPolicies);

  equal(zero.key, 'credit:credit-zero@v1');
  equal(zero.values.identity, 'credit-zero@v1');
  equal(zero.values.legalEntity, 'legal-1');
  equal(zero.values.authorityLevel, 'level-commercial');
  equal(zero.values.chargeType, 'charge-freight');
  equal(zero.values.limit, '金额 0（最小货币单位）');
  equal(zero.values.effective, '2026-01-01 00:00:00 UTC → 持续有效');
});

test('比例额度按基点换算成百分比，并带上结束时点', () => {
  const [, ratio] = rowsOf(creditPolicies);

  equal(ratio.values.limit, '比例 15%');
  equal(ratio.values.effective, '2026-01-01 00:00:00 UTC → 2026-01-01 01:00:00 UTC');
  equal(ratio.values.registeredAt, '2026-01-01 00:00:00 UTC');
});

test('额度两键都缺是响应不合契约，如实点名而不是显示成零或空', () => {
  const [row] = rowsOf({
    ...creditPolicies,
    policies: [{ ...creditPolicies.policies[0], limitMinor: undefined }],
  });

  equal(row.values.limit, '额度缺失（响应不合契约）');
});

// 抽出行转写时顺带钉住取消授权的三态：这三句是页面替目录说的话，抽出来之后不能变。
test('取消授权按请求方三态作答：未声明 / 允许 / 不许取消', () => {
  const declared = {
    objectId: 'authz-1',
    version: 'v1',
    scope: 'SYN-SCOPE',
    status: 'EFFECTIVE',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
    publishedAt: '2026-01-01T00:00:00Z',
    cancellationAuthorityDeclared: true,
    cancellationAuthorities: [{ party: 'CUSTOMER', ruleReference: 'rule-c1' }],
  };

  equal(cancellationCell(declared, 'CUSTOMER'), '允许:rule-c1');
  equal(cancellationCell(declared, 'OPERATIONS'), '不许取消');
  equal(
    cancellationCell({ ...declared, cancellationAuthorityDeclared: false, cancellationAuthorities: [] }, 'CUSTOMER'),
    '未声明',
  );
});
