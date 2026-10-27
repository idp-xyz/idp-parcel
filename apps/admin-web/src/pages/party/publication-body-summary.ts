// 发布前给人读的「正文要点」（票 party-commercial-uiux/19）。
//
// 「将要发布」卡（publication-summary.ts）只复述版本信息；正文各册形状不同，格子又多，发布不能撤回，人在按下去之前最该
// 再读一遍的恰恰是正文里写了什么。这里从表单此刻组出的那份载荷里，按册取出几条白话要点：谁、对什么、怎么约定。它只是
// 复述，不判任何门、不替服务端校验；表单格子、预览逐格问题与内容摘要都照旧。
//
// 编码一律换成词表里的中文，词表里没有的原样显示（服务端新增一格时宁可露出原词，也不归进已有的说法）。
// 纯函数，node:test 直接跑。

import { formatInstant } from '../moment';
import {
  cancellationPartyLabels,
  claimDeadlineKindLabels,
  commercialDirectionLabels,
  controlFailureDispositionLabels,
  controlKindLabels,
  controlRequirementLabels,
  creditRatioBaseLabels,
  jointPassConditionLabels,
  labelOf,
  planBindingConversionLabels,
  settlementMethodLabels,
  taxDispositionLabels,
} from './presentation';
import type {
  AcceptanceRulePackageBodyPayload,
  CommercialPublicationPayload,
  CreditPolicyBodyPayload,
  CustomerContractBodyPayload,
  CustomerServiceRuleBodyPayload,
  DeliveryConditionPayload,
  PreAcceptanceFinancialControlPolicyBodyPayload,
  PricePolicyBodyPayload,
  SettlementPolicyBodyPayload,
  SupplierAgreementBodyPayload,
} from './publication-draft-api';
import type { PublicationFact } from './publication-summary';

const none = '无';

function listOrNone(items: readonly string[] | undefined): string {
  return items && items.length > 0 ? items.join('、') : none;
}

function range(startsAt: string, endsAt: string | undefined, timeZone: string): string {
  return `${formatInstant(startsAt, { timeZone })} → ${endsAt ? formatInstant(endsAt, { timeZone }) : '持续有效'}`;
}

function deliveryConditionFacts(dc: DeliveryConditionPayload | undefined, layer: string): PublicationFact[] {
  if (!dc) return [{ label: layer, value: '未声明（这一版没有这一层的交付条件）' }];
  const facts: PublicationFact[] = [
    { label: `${layer}：交付方式`, value: listOrNone(dc.methods) },
    { label: `${layer}：收件范围规则`, value: dc.recipientScopeRule },
    { label: `${layer}：交付证明规则`, value: dc.proofOfDeliveryRule },
  ];
  if (dc.tightens) facts.push({ label: `${layer}：收紧的产品版本`, value: `${dc.tightens.objectId} ${dc.tightens.version}` });
  return facts;
}

function contractFacts(body: CustomerContractBodyPayload | undefined): PublicationFact[] {
  if (!body) return [{ label: '合同正文', value: '未填' }];
  const { contractContent, preAcceptanceControl } = body;
  const bindings = (contractContent.bindings ?? []).map((binding) =>
    binding.policy
      ? `${binding.chargeScope} → 控制策略 ${binding.policy}`
      : `${binding.chargeScope} → 不适用（依据：${binding.inapplicabilityBasis ?? '未填'}）`,
  );
  const facts: PublicationFact[] = [
    { label: '货主客户账户', value: contractContent.customerAccount },
    { label: '接单规则包', value: contractContent.rulePackage },
    { label: '费用范围约定', value: bindings.length > 0 ? bindings.join('；') : '没有对任何费用范围作约定' },
  ];
  if (preAcceptanceControl) {
    const basis = preAcceptanceControl.notApplicableBasis ? `（不适用依据：${preAcceptanceControl.notApplicableBasis}）` : '';
    facts.push({
      label: '合同级受理前财务控制',
      value: `${labelOf(controlRequirementLabels, preAcceptanceControl.requirement)}${basis}`,
    });
  }
  return [...facts, ...deliveryConditionFacts(body.deliveryConditions, '合同层交付条件')];
}

function agreementFacts(body: SupplierAgreementBodyPayload | undefined, timeZone: string): PublicationFact[] {
  if (!body) return [{ label: '协议正文', value: '未填' }];
  return [
    { label: '供应商', value: body.supplier },
    { label: '责任法人', value: body.legalEntity },
    { label: '采购方案', value: body.purchasePlan },
    { label: '协议适用范围', value: body.scope },
    { label: '协议有效区间', value: range(body.effectiveStartsAt, body.effectiveEndsAt, timeZone) },
  ];
}

function rulePackageFacts(body: AcceptanceRulePackageBodyPayload | undefined, timeZone: string): PublicationFact[] {
  if (!body) return [{ label: '规则包正文', value: '未填' }];
  const main = body.rulePackageBody;
  const rules = main.rules ?? [];
  const byCategory = new Map<string, number>();
  for (const rule of rules) byCategory.set(rule.category, (byCategory.get(rule.category) ?? 0) + 1);
  const sections: [string, boolean][] = [
    ['接受判断时点策略', (body.asOfPolicies ?? []).length > 0],
    ['接受内容', body.acceptanceContent !== undefined],
    ['收寄来源与资质', body.intakeQualification !== undefined],
    ['终局规则', (body.finalRules ?? []).length > 0],
    ['面单有效期', body.finalRuleValidity !== undefined],
    ['资料修订允许', body.sourceDataAmendment !== undefined],
  ];
  return [
    { label: '服务产品', value: main.serviceProduct },
    { label: '客户合同', value: main.contract },
    { label: '责任法人', value: main.legalEntity },
    { label: '规则适用范围', value: main.scope },
    { label: '规则适用区间', value: range(main.effectiveStartsAt, main.effectiveEndsAt, timeZone) },
    {
      label: '规则引用',
      value:
        rules.length === 0
          ? '没有引用任何规则'
          : `共 ${rules.length} 条：${[...byCategory].map(([category, count]) => `${category} ${count} 条`).join('、')}`,
    },
    { label: '已声明的内容', value: listOrNone(sections.filter(([, declared]) => declared).map(([name]) => name)) },
    {
      label: '未声明的内容',
      value: `${listOrNone(sections.filter(([, declared]) => !declared).map(([name]) => name))}（整节留空即未声明，不是「无」）`,
    },
  ];
}

function controlPolicyFacts(body: PreAcceptanceFinancialControlPolicyBodyPayload | undefined): PublicationFact[] {
  if (!body) return [{ label: '策略正文', value: '未填' }];
  return [
    { label: '共同通过条件', value: labelOf(jointPassConditionLabels, body.jointPassCondition) },
    ...body.controls.map((item, index) => ({
      label: `控制项 ${index + 1}`,
      value:
        `${item.order === undefined ? '' : `第 ${item.order} 位：`}${labelOf(controlKindLabels, item.control)} × 费用范围 ${item.chargeScope}` +
        `；不通过则${labelOf(controlFailureDispositionLabels, item.onFailure)}；责任 ${item.responsibility}`,
    })),
  ];
}

function priceFacts(body: PricePolicyBodyPayload | undefined, timeZone: string): PublicationFact[] {
  if (!body) return [{ label: '价格政策正文', value: '未填' }];
  const facts: PublicationFact[] = [
    { label: '政策方向', value: labelOf(commercialDirectionLabels, body.direction) },
    { label: '价格方案', value: `${body.pricingPlan}（方案方向 ${labelOf(commercialDirectionLabels, body.planDirection)}）` },
    { label: '方案转换', value: labelOf(planBindingConversionLabels, body.conversion) },
    { label: '政策适用范围', value: body.scope },
    { label: '政策有效区间', value: range(body.effectiveStartsAt, body.effectiveEndsAt, timeZone) },
  ];
  if (body.caliber) {
    const tax = labelOf(taxDispositionLabels, body.caliber.taxDisposition);
    facts.push({ label: '价格口径', value: body.caliber.taxClassification ? `${tax}（${body.caliber.taxClassification}）` : tax });
    if (body.caliber.fx) {
      facts.push({
        label: '外币口径',
        value:
          `报价类型 ${body.caliber.fx.quoteType}；取值时点 ${body.caliber.fx.asOfSemantics}；` +
          `时点策略版本 ${body.caliber.fx.asOfPolicyVersion}`,
      });
    }
  } else {
    facts.push({ label: '价格口径', value: '未声明' });
  }
  return facts;
}

function settlementFacts(body: SettlementPolicyBodyPayload | undefined, timeZone: string): PublicationFact[] {
  if (!body) return [{ label: '结算政策正文', value: '未填' }];
  return [
    { label: '结算方式', value: labelOf(settlementMethodLabels, body.method) },
    { label: '责任法人', value: body.legalEntity },
    { label: '相对方', value: body.counterparty },
    { label: '客户合同', value: `${body.contract.objectId} ${body.contract.version}` },
    { label: '费用范围', value: body.chargeScope },
    { label: '币种', value: body.currency },
    { label: '政策有效区间', value: range(body.effectiveStartsAt, body.effectiveEndsAt, timeZone) },
  ];
}

function creditLimit(body: CreditPolicyBodyPayload): string {
  if (body.limitMinor !== undefined) return `${body.limitMinor}（最小货币单位）`;
  if (body.limitRatioBasisPoints !== undefined) {
    const percent = body.limitRatioBasisPoints / 100;
    return `${percent}%（基数：${body.ratioBase ? labelOf(creditRatioBaseLabels, body.ratioBase) : '未填'}）`;
  }
  return '未填';
}

function creditFacts(body: CreditPolicyBodyPayload | undefined, timeZone: string): PublicationFact[] {
  if (!body) return [{ label: '信用政策正文', value: '未填' }];
  return [
    { label: '责任法人', value: body.legalEntity },
    { label: '授权层级', value: body.authorityLevel },
    { label: '费用类型', value: body.chargeType },
    { label: '额度', value: creditLimit(body) },
    { label: '政策有效区间', value: range(body.effectiveStartsAt, body.effectiveEndsAt, timeZone) },
  ];
}

function serviceRuleFacts(body: CustomerServiceRuleBodyPayload | undefined): PublicationFact[] {
  if (!body) return [{ label: '规则正文', value: '未填' }];
  const target = body.serviceProduct ? `服务产品 ${body.serviceProduct}` : body.customerContract ? `客户合同 ${body.customerContract}` : '未填';
  return [
    { label: '适用对象', value: target },
    { label: '责任方', value: body.responsible },
    { label: '规则适用范围', value: body.scope },
    {
      label: '索赔期限',
      value:
        body.claimDeadlines.length === 0
          ? none
          : body.claimDeadlines
              .map(
                (row) =>
                  `${labelOf(claimDeadlineKindLabels, row.kind)}：自「${row.startEvent}」起 ${row.days ?? '—'} 天（日历 ${row.calendar}）`,
              )
              .join('；'),
    },
    {
      label: '最低材料',
      value:
        body.minimumMaterials.length === 0
          ? none
          : body.minimumMaterials.map((row) => `${row.claimKind}：${listOrNone(row.materials)}`).join('；'),
    },
  ];
}

/** 按册取这一份载荷的正文要点；没有可复述的返回空数组。 */
export function publicationBodySummary(payload: CommercialPublicationPayload, timeZone: string): PublicationFact[] {
  switch (payload.kind) {
    case 'SERVICE_PRODUCT':
      return deliveryConditionFacts(payload.serviceProduct?.deliveryConditions, '产品层交付条件');
    case 'CUSTOMER_CONTRACT':
      return contractFacts(payload.customerContract);
    case 'SUPPLIER_AGREEMENT':
      return agreementFacts(payload.supplierAgreement, timeZone);
    case 'ACCEPTANCE_RULE_PACKAGE':
      return rulePackageFacts(payload.acceptanceRulePackage, timeZone);
    case 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY':
      return controlPolicyFacts(payload.preAcceptanceFinancialControlPolicy);
    case 'PRICE_RULE':
      return priceFacts(payload.pricePolicy, timeZone);
    case 'SETTLEMENT_POLICY':
      return settlementFacts(payload.settlementPolicy, timeZone);
    case 'CREDIT_POLICY':
      return creditFacts(payload.creditPolicy, timeZone);
    case 'AUTHORIZATION_RULE':
      return [
        {
          label: '取消授权',
          value:
            (payload.authorizationRule?.cancellationAuthority ?? []).length === 0
              ? none
              : (payload.authorizationRule?.cancellationAuthority ?? [])
                  .map((row) => `${labelOf(cancellationPartyLabels, row.party)}：${row.rule}`)
                  .join('；'),
        },
      ];
    case 'CUSTOMER_SERVICE_RULE':
      return serviceRuleFacts(payload.customerServiceRule);
  }
}
