import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { CommercialPolicyListResponseBody } from './api';
import { cancellationCell, kindColumns, pricePolicyCaliberCell, rowsOf } from './policy-rows';
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

// 票 admin-write-faces/06（ADR-0115）：接受前财务控制策略版本从此有册可看。钉三件：chip 词表与列集都有它；
// 「谁喂它」那一句把两本近名的册互相点名（策略册 ← PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 版本，声明册 ←
// 挂 CUSTOMER_CONTRACT 版本的声明），且票 03 那句「今天没有册可看」从提示句与声明册那一句里都退场了。
test('接受前财务控制策略册：chip 词表、列集与「谁喂它」都有它，「没有册可看」退场', () => {
  ok(commercialPolicyKinds.includes('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY'));
  equal(policyKindLabels.PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY, '接受前财务控制策略');
  deepEqual(
    kindColumns.PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY.map((column) => column.header),
    [
      '策略对象 / 版本',
      '适用范围',
      '生命周期状态',
      '正文',
      '共同通过条件',
      '控制项(顺序. 种类@费用范围 → 失败处置;责任)',
      '有效区间',
      '发布时间',
    ],
  );
  ok(policyKindSources.PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY.includes('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY'));
  ok(policyKindSources.PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY.includes('接受前财务控制」册'));
  ok(policyKindSources.PRE_ACCEPTANCE_CONTROL.includes('接受前财务控制策略」册'));
  ok(!policyKindSources.PRE_ACCEPTANCE_CONTROL.includes('没有册可看'));
  ok(!registrationSnapshotHints.publication.includes('没有册可看'));
  ok(registrationSnapshotHints.publication.includes('接受前财务控制策略册'));
});

// 输入照后端 query_commercial_catalogue_test.go 里
// TestPoliciesEndpointTranscribesControlPolicyContentOnlyWhenRegistered 钉住的两行：只有壳、带两项组合正文。
const controlPolicies: CommercialPolicyListResponseBody = {
  outcome: 'COMMERCIAL_POLICIES_LISTED',
  kind: 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
  policies: [
    {
      objectId: 'fcp-bare',
      version: 'v1',
      scope: 'scope-1',
      status: 'EFFECTIVE',
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      publishedAt: '2026-01-01T00:00:00Z',
      contentRegistered: false,
    },
    {
      objectId: 'fcp-full',
      version: 'v2',
      scope: 'scope-1',
      status: 'EFFECTIVE',
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      effectiveEndsAt: '2026-01-01T01:00:00Z',
      publishedAt: '2026-01-01T00:00:00Z',
      contentRegistered: true,
      content: {
        jointPassCondition: 'ALL_CONTROLS_PASS',
        registeredAt: '2026-01-01T00:01:00Z',
        controls: [
          { control: 'PREPAID_FREEZE', chargeScope: 'charge-scope-a', order: 1, onFailure: 'REJECT', responsibility: 'customer-1' },
          { control: 'CREDIT_CHECK', chargeScope: 'charge-scope-a', order: 2, onFailure: 'AUTHORIZED_DISPOSITION', responsibility: 'operator-legal-1' },
        ],
      },
    },
  ],
};

test('只有壳的策略版本照列为「未登记」，不从目录上消失', () => {
  const [bare] = rowsOf(controlPolicies);

  equal(bare.key, 'control-policy:fcp-bare@v1');
  equal(bare.values.identity, 'fcp-bare@v1');
  equal(bare.values.status, '已生效');
  equal(bare.values.contentRegistered, '未登记');
  equal(bare.values.jointPassCondition, '—');
  equal(bare.values.controls, '未登记正文');
  equal(bare.values.effective, '2026-01-01 00:00:00 UTC → 持续有效');
});

test('登了正文的策略按判断顺序逐项列出控制项，共同通过条件配中文', () => {
  const [, full] = rowsOf(controlPolicies);

  equal(full.values.contentRegistered, '已登记(2026-01-01 00:01:00 UTC)');
  equal(full.values.jointPassCondition, '全部控制通过');
  equal(
    full.values.controls,
    '1. 预付冻结@charge-scope-a → 拒绝;责任:customer-1 | 2. 信用校验@charge-scope-a → 进入授权处置;责任:operator-legal-1',
  );
  equal(full.values.effective, '2026-01-01 00:00:00 UTC → 2026-01-01 01:00:00 UTC');
});

test('布尔说已登记而正文节缺了是响应不合契约，如实点名而不是显示成空', () => {
  const [odd] = rowsOf({
    ...controlPolicies,
    policies: [{ ...controlPolicies.policies[0], objectId: 'fcp-odd', contentRegistered: true }],
  } as CommercialPolicyListResponseBody);

  equal(odd.values.contentRegistered, '正文缺失(响应不合契约)');
  equal(odd.values.controls, '正文缺失(响应不合契约)');
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

// 输入照后端 query_commercial_catalogue_test.go 里价格政策册那三行的形状（票 admin-write-faces/14「结果在同册立刻可见（含口径列）」）：
// 带汇率的销售口径、不涉外币且税务不适用的采购口径、只有正文没有口径的行。三处可缺的键缺席即「没有」，不是空串。
const pricePolicies: CommercialPolicyListResponseBody = {
  outcome: 'COMMERCIAL_POLICIES_LISTED',
  kind: 'PRICE_POLICY',
  policies: [
    {
      objectId: 'price-fx',
      version: 'v1',
      direction: 'SELL',
      planRef: 'PLAN-CN-SG-SELL@v1',
      planDirection: 'BUY',
      bindingConversion: 'FROZEN_BUY_EVALUATION',
      policyScope: 'pricing-scope-1',
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      registeredAt: '2026-01-01T00:00:00Z',
      caliberDeclared: true,
      caliber: {
        taxDisposition: 'TAX_EXCLUSIVE',
        taxClassification: 'vat-standard',
        volumetricFactor: 'sell-divisor-5000-cm',
        fx: { quoteType: 'boc-cash-selling', asOfSemantics: 'AT_ORDER_DATE', asOfPolicyVersion: 'asof-policy/v3' },
        registeredAt: '2026-01-01T00:00:00Z',
      },
    },
    {
      objectId: 'price-plain',
      version: 'v1',
      direction: 'BUY',
      planRef: 'PLAN-CN-SG-COST@v1',
      planDirection: 'BUY',
      bindingConversion: 'NONE',
      policyScope: 'pricing-scope-1',
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      registeredAt: '2026-01-01T00:00:00Z',
      caliberDeclared: true,
      caliber: { taxDisposition: 'TAX_NOT_APPLICABLE', registeredAt: '2026-01-01T00:00:00Z' },
    },
    {
      objectId: 'price-bare',
      version: 'v1',
      direction: 'SELL',
      planRef: 'PLAN-CN-SG-SELL@v1',
      planDirection: 'SELL',
      bindingConversion: 'NONE',
      policyScope: 'pricing-scope-1',
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      registeredAt: '2026-01-01T00:00:00Z',
      caliberDeclared: false,
    },
  ],
};

test('商业价格政策册多一列口径：三段各显自己的真话，没口径的行列为「未登记」而不是空', () => {
  deepEqual(
    kindColumns.PRICE_POLICY.map((column) => column.header),
    ['政策对象 / 版本', '政策方向', '定价方案引用', '方案方向', '绑定转换', '适用范围', '计价口径（税务;体积;汇率）', '有效区间', '登记时间'],
  );
  const [withFx, plain, bare] = rowsOf(pricePolicies);
  equal(withFx.key, 'price:price-fx@v1');
  equal(withFx.values.direction, '卖价');
  equal(withFx.values.planDirection, '买价');
  equal(withFx.values.caliber, '税务 未税@vat-standard;体积 sell-divisor-5000-cm;汇率 boc-cash-selling/AT_ORDER_DATE/asof-policy/v3');
  equal(plain.values.caliber, '税务 税务不适用;体积 无系数（非销售方向）;汇率 未声明');
  equal(bare.values.caliber, '未登记');
  equal(pricePolicyCaliberCell({ ...pricePolicies.policies[0], caliber: undefined }), '口径缺失（响应不合契约）');
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
