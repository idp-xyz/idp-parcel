import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { publicationBodySummary } from './publication-body-summary';
import type { CommercialObjectKindName, CommercialPublicationPayload } from './publication-draft-api';

const tz = 'Asia/Shanghai';

function payload(kind: CommercialObjectKindName, extra: Partial<CommercialPublicationPayload>): CommercialPublicationPayload {
  return { kind, objectId: 'OBJ', version: 'v1', scope: 'S', effectiveStartsAt: '2026-01-01T00:00:00Z', ...extra };
}

const valueOf = (facts: { label: string; value: string }[], label: string): string | undefined =>
  facts.find((fact) => fact.label === label)?.value;

test('每一册都给出要点，标签不重复（卡片按标签做键）', () => {
  const kinds: CommercialObjectKindName[] = [
    'SERVICE_PRODUCT',
    'CUSTOMER_CONTRACT',
    'SUPPLIER_AGREEMENT',
    'ACCEPTANCE_RULE_PACKAGE',
    'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
    'PRICE_RULE',
    'SETTLEMENT_POLICY',
    'CREDIT_POLICY',
    'AUTHORIZATION_RULE',
    'CUSTOMER_SERVICE_RULE',
  ];
  for (const kind of kinds) {
    const facts = publicationBodySummary(payload(kind, {}), tz);
    ok(facts.length > 0, `${kind} 没有要点`);
    equal(new Set(facts.map((fact) => fact.label)).size, facts.length, `${kind} 标签重复`);
  }
});

test('服务产品：没有交付条件说「未声明」，有则列方式与两条规则', () => {
  equal(valueOf(publicationBodySummary(payload('SERVICE_PRODUCT', {}), tz), '产品层交付条件')?.startsWith('未声明'), true);
  const facts = publicationBodySummary(
    payload('SERVICE_PRODUCT', {
      serviceProduct: { deliveryConditions: { methods: ['A', 'B'], recipientScopeRule: 'R1', proofOfDeliveryRule: 'R2' } },
    }),
    tz,
  );
  equal(valueOf(facts, '产品层交付条件：交付方式'), 'A、B');
  equal(valueOf(facts, '产品层交付条件：交付证明规则'), 'R2');
});

test('客户合同：约定行按「策略 / 不适用 + 依据」复述，合同级控制翻成中文', () => {
  const facts = publicationBodySummary(
    payload('CUSTOMER_CONTRACT', {
      customerContract: {
        contractContent: {
          customerAccount: 'ACC-1',
          rulePackage: 'RP-1',
          bindings: [
            { chargeScope: 'COD', policy: 'FC-1' },
            { chargeScope: 'FREIGHT', inapplicabilityBasis: 'BASIS-1' },
          ],
        },
        preAcceptanceControl: { requirement: 'NOT_APPLICABLE', notApplicableBasis: 'B2' },
      },
    }),
    tz,
  );
  equal(valueOf(facts, '货主客户账户'), 'ACC-1');
  equal(valueOf(facts, '费用范围约定'), 'COD → 控制策略 FC-1；FREIGHT → 不适用（依据：BASIS-1）');
  equal(valueOf(facts, '合同级受理前财务控制'), '不适用（不适用依据：B2）');
});

test('供应商协议、结算政策、信用政策：关键格与区间（显示时区加持续有效）', () => {
  const agreement = publicationBodySummary(
    payload('SUPPLIER_AGREEMENT', {
      supplierAgreement: { supplier: 'SUP', legalEntity: 'LE', scope: 'AS', purchasePlan: 'PLAN@1', effectiveStartsAt: '2026-01-01T00:00:00Z' },
    }),
    tz,
  );
  equal(valueOf(agreement, '采购方案'), 'PLAN@1');
  equal(valueOf(agreement, '协议有效区间')?.endsWith('持续有效'), true);
  const settlement = publicationBodySummary(
    payload('SETTLEMENT_POLICY', {
      settlementPolicy: {
        method: 'PREPAID',
        legalEntity: 'LE',
        counterparty: 'CP',
        contract: { objectId: 'CC', version: 'v2' },
        chargeScope: 'FREIGHT',
        currency: 'CNY',
        effectiveStartsAt: '2026-01-01T00:00:00Z',
      },
    }),
    tz,
  );
  equal(valueOf(settlement, '结算方式'), '预付');
  equal(valueOf(settlement, '客户合同'), 'CC v2');
  const amount = publicationBodySummary(
    payload('CREDIT_POLICY', { creditPolicy: { legalEntity: 'LE', authorityLevel: 'L1', chargeType: 'T', limitMinor: 0, effectiveStartsAt: '2026-01-01T00:00:00Z' } }),
    tz,
  );
  equal(valueOf(amount, '额度'), '0（最小货币单位）');
  const ratio = publicationBodySummary(
    payload('CREDIT_POLICY', {
      creditPolicy: {
        legalEntity: 'LE',
        authorityLevel: 'L1',
        chargeType: 'T',
        limitRatioBasisPoints: 1250,
        ratioBase: 'POSTED_BALANCE',
        effectiveStartsAt: '2026-01-01T00:00:00Z',
      },
    }),
    tz,
  );
  equal(valueOf(ratio, '额度'), '12.5%（基数：入账余额）');
});

test('接单规则包：规则按分类计数，已声明与未声明的节分开列，并提醒留空不是「无」', () => {
  const facts = publicationBodySummary(
    payload('ACCEPTANCE_RULE_PACKAGE', {
      acceptanceRulePackage: {
        rulePackageBody: {
          serviceProduct: 'P',
          contract: 'C',
          legalEntity: 'LE',
          scope: 'S',
          effectiveStartsAt: '2026-01-01T00:00:00Z',
          rules: [
            { category: 'ROUTE', reference: 'r1' },
            { category: 'ROUTE', reference: 'r2' },
            { category: 'PRICE', reference: 'r3' },
          ],
        },
        asOfPolicies: [{ judgment: 'J', semantics: 'S', policyVersion: 'V' }],
        finalRules: [{ outcome: 'EFFECTIVE_DELIVERY', finalKind: 'K' }],
      },
    }),
    tz,
  );
  equal(valueOf(facts, '规则引用'), '共 3 条：ROUTE 2 条、PRICE 1 条');
  equal(valueOf(facts, '已声明的内容'), '接受判断时点策略、终局规则');
  equal(valueOf(facts, '未声明的内容')?.includes('面单有效期'), true);
  equal(valueOf(facts, '未声明的内容')?.includes('不是「无」'), true);
});

test('接受前财务控制策略、授权规则、客户服务规则：逐行复述，编码换成中文', () => {
  const control = publicationBodySummary(
    payload('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY', {
      preAcceptanceFinancialControlPolicy: {
        jointPassCondition: 'ALL_CONTROLS_PASS',
        controls: [{ control: 'CREDIT_CHECK', chargeScope: 'COD', order: 1, onFailure: 'REJECT', responsibility: 'RESP' }],
      },
    }),
    tz,
  );
  equal(valueOf(control, '共同通过条件'), '全部控制通过');
  equal(valueOf(control, '控制项 1'), '第 1 位：信用校验 × 费用范围 COD；不通过则拒绝；责任 RESP');
  const auth = publicationBodySummary(
    payload('AUTHORIZATION_RULE', { authorizationRule: { cancellationAuthority: [{ party: 'CUSTOMER', rule: 'RULE-1' }, { party: 'OPERATIONS', rule: 'RULE-2' }] } }),
    tz,
  );
  equal(valueOf(auth, '取消授权'), '客户：RULE-1；运营：RULE-2');
  const rule = publicationBodySummary(
    payload('CUSTOMER_SERVICE_RULE', {
      customerServiceRule: {
        serviceProduct: 'PROD',
        responsible: 'WHO',
        scope: 'S',
        claimDeadlines: [{ kind: 'FIRST_CLAIM', startEvent: 'DELIVERED', days: 14, calendar: 'CAL' }],
        minimumMaterials: [{ claimKind: 'LOSS', materials: ['POD', 'INVOICE'] }],
      },
    }),
    tz,
  );
  equal(valueOf(rule, '适用对象'), '服务产品 PROD');
  equal(valueOf(rule, '索赔期限'), '客户首次索赔期限：自「DELIVERED」起 14 天（日历 CAL）');
  equal(valueOf(rule, '最低材料'), 'LOSS：POD、INVOICE');
});

test('价格政策：方向与转换翻成中文；没有口径节说「未声明」，有则列含税口径', () => {
  const base = {
    direction: 'SELL',
    pricingPlan: 'PLAN@1',
    planDirection: 'BUY',
    conversion: 'FROZEN_BUY_EVALUATION',
    scope: 'S',
    effectiveStartsAt: '2026-01-01T00:00:00Z',
  };
  const without = publicationBodySummary(payload('PRICE_RULE', { pricePolicy: base }), tz);
  equal(valueOf(without, '政策方向'), '卖价');
  equal(valueOf(without, '价格口径'), '未声明');
  const withCaliber = publicationBodySummary(
    payload('PRICE_RULE', { pricePolicy: { ...base, caliber: { taxDisposition: 'TAX_INCLUSIVE', taxClassification: 'VAT6' } } }),
    tz,
  );
  deepEqual(valueOf(withCaliber, '价格口径'), '含税（VAT6）');
});
